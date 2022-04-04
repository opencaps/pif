package dbusconn

import (
	"bytes"
	"sync"
	"time"

	"github.com/godbus/dbus/v5"
	"github.com/godbus/dbus/v5/prop"
	"github.com/op/go-logging"
)

const (
	msgBodyNotValid     = "body not valid"
	signalDeviceAdded   = "DeviceAdded"
	signalDeviceRemoved = "DeviceRemoved"

	propertyOperabilityState  = "OperabilityState"
	propertyPairingState      = "PairingState"
	propertyReachabilityState = "ReachabilityState"
	propertyVersion           = "Version"
	propertyOptions           = "Options"

	// OperabilityOk state 'ok' for OperabilityState
	OperabilityOk OperabilityState = "OK"
	// OperabilityPartial state 'partial' for OperabilityState
	OperabilityPartial OperabilityState = "PARTIAL"
	// OperabilityKo state 'Ko' for OperabilityState
	OperabilityKo OperabilityState = "KO"
	// OperabilityUnknown state 'unknown' for OperabilityState
	OperabilityUnknown OperabilityState = "UNKNOWN"

	// PairingOk state 'ok' for PairingState
	PairingOk PairingState = "OK"
	// PairingInProgres state 'in progress' for PairingState
	PairingInProgress PairingState = "IN_PROGRESS"
	// PairingKo state 'ko' for PairingState
	PairingKo PairingState = "KO"
	// PairingUnknown state 'unknown' for PairingState
	PairingUnknown PairingState = "UNKNOWN"
	// PairingNotNeeded state 'not needed' for PairingState
	PairingNotNeeded PairingState = "NOT_NEEDED"

	// ReachabilityOk state 'ok' for ReachabilityState
	ReachabilityOk ReachabilityState = "OK"
	// ReachabilityKo state 'ko' for ReachabilityState
	ReachabilityKo ReachabilityState = "KO"
	// ReachabilityRescue state 'rescue' for ReachabilityState
	ReachabilityRescue ReachabilityState = "RESCUE"
	// ReachabilityUnknown state 'unknown' for ReachabilityState
	ReachabilityUnknown ReachabilityState = "UNKNOWN"
)

type setDeviceOptionInterface interface {
	SetDeviceOptions(*Device) *dbus.Error
}

type updateFirmwareInterface interface {
	UpdateFirmware(*Device, string) (string, *dbus.Error)
}

type reacheabilityTimeoutInterface interface {
	ReachabilityWentKo(*Device)
}

// Device object structure
type Device struct {
	sync.Mutex

	protocol *Protocol

	DevID           string
	Address         string
	TypeID          string
	TypeVersion     string
	Options         []byte
	FirmwareVersion string
	Operability     OperabilityState
	PairingState    PairingState
	Reachability    ReachabilityState

	ReachabilityTimeout time.Duration

	timer *time.Timer

	properties *prop.Properties
	Items      map[string]*Item

	log *logging.Logger

	SetDeviceOptionCb      setDeviceOptionInterface
	UpdateFirmwareCb       updateFirmwareInterface
	ReacheabilityTimeoutCB reacheabilityTimeoutInterface
}

// OperabilityState informs if the device work
type OperabilityState string

// PairingState informs the state of the pairing
type PairingState string

// ReachabilityState informs if the device is reachable
type ReachabilityState string

func (d *Device) setDeviceOptions(c *prop.Change) *dbus.Error {
	if !isNil(d.SetDeviceOptionCb) {
		return d.SetDeviceOptionCb.SetDeviceOptions(d)
	}
	d.log.Warning("No Options")
	return nil
}

//UpdateFirmware is the dbus method to update the firmware of the device
func (d *Device) UpdateFirmware(data string) (string, *dbus.Error) {
	if !isNil(d.UpdateFirmwareCb) {
		return d.UpdateFirmwareCb.UpdateFirmware(d, data)
	}
	d.log.Warning("Update firmware not implemented")
	return "", nil
}

func (d *Device) reachabilityCBTimeout() {
	d.SetReachabilityState(ReachabilityKo)

	if !isNil(d.ReacheabilityTimeoutCB) {
		d.ReacheabilityTimeoutCB.ReachabilityWentKo(d)
	}
}

func initDevice(devID string, address string, typeID string, typeVersion string, options []byte, p *Protocol) *Device {
	return &Device{
		DevID:        devID,
		Address:      address,
		TypeID:       typeID,
		TypeVersion:  typeVersion,
		Options:      options,
		Reachability: ReachabilityKo,
		PairingState: PairingUnknown,
		Operability:  OperabilityUnknown,
		Items:        make(map[string]*Item),
		protocol:     p,
		log:          p.log,
	}
}

func (dc *Dbus) emitDeviceAdded(device *Device) {
	args := make([]interface{}, 4)
	args[0] = device.Address
	args[1] = device.TypeID
	args[2] = device.TypeVersion
	args[3] = device.Options
	path := dbus.ObjectPath(dbusPathPrefix + dc.ProtocolName + "/" + device.DevID)
	dc.conn.Emit(path, dbusDeviceInterface+"."+signalDeviceAdded, args...)
}

func (dc *Dbus) emitDeviceRemoved(devID string) {
	path := dbus.ObjectPath(dbusPathPrefix + dc.ProtocolName + "/" + devID)
	dc.conn.Emit(path, dbusDeviceInterface+"."+signalDeviceRemoved)
}

func (dc *Dbus) exportDeviceOnDbus(device *Device) {
	if dc.conn == nil {
		dc.Log.Warning("Unable to export dbus object because dbus connection nil")
	}

	path := dbus.ObjectPath(dbusPathPrefix + dc.ProtocolName + "/" + device.DevID)

	// properties
	propsSpec := initDeviceProp(device)
	properties, err := prop.Export(dc.conn, path, propsSpec)
	if err == nil {
		device.properties = properties
	} else {
		dc.Log.Error("Fail to export the properties of the device", device.DevID, err)
	}

	// object
	dc.conn.Export(device, path, dbusDeviceInterface)

	dc.Log.Info("Device exported:", path)
}

// AddItem adds a new item to device
func (device *Device) AddItem(itemID string, typeID string, typeVersion string, options []byte) (bool, *dbus.Error) {
	device.log.Info("AddItem called - itemID:", itemID, "typeID:", typeID, "typeVersion:", typeVersion, "options:", options)

	device.Lock()
	_, itemPresent := device.Items[itemID]
	if !itemPresent {
		item := initItem(itemID, typeID, typeVersion, options, device)
		device.Items[itemID] = item
		device.protocol.dc.exportItemOnDbus(device.DevID, item)

		if !isNil(device.protocol.Callbacks) {
			go device.protocol.Callbacks.AddItem(item)
		}
		device.protocol.dc.emitItemAdded(device.DevID, item)
		device.Unlock()
		return false, nil
	}
	device.Unlock()
	return true, nil
}

// RemoveItem remove item from device
func (device *Device) RemoveItem(itemID string) *dbus.Error {
	device.log.Info("RemoveItem called - itemID:", itemID)

	device.Lock()
	_, present := device.Items[itemID]
	if present {
		delete(device.Items, itemID)
		if !isNil(device.protocol.Callbacks) {
			go device.protocol.Callbacks.RemoveItem(device.DevID, itemID)
		}
		device.protocol.dc.emitItemRemoved(device.DevID, itemID)
	}
	device.Unlock()
	return nil
}

func initDeviceProp(device *Device) map[string]map[string]*prop.Prop {
	return map[string]map[string]*prop.Prop{
		dbusDeviceInterface: {
			propertyOperabilityState: {
				Value:    device.Operability,
				Writable: false,
				Emit:     prop.EmitTrue,
				Callback: nil,
			},
			propertyPairingState: {
				Value:    device.PairingState,
				Writable: false,
				Emit:     prop.EmitTrue,
				Callback: nil,
			},
			propertyReachabilityState: {
				Value:    device.Reachability,
				Writable: false,
				Emit:     prop.EmitTrue,
				Callback: nil,
			},
			propertyVersion: {
				Value:    device.FirmwareVersion,
				Writable: false,
				Emit:     prop.EmitTrue,
				Callback: nil,
			},
			propertyOptions: {
				Value:    device.Options,
				Writable: true,
				Emit:     prop.EmitTrue,
				Callback: device.setDeviceOptions,
			},
		},
	}
}

// SetOperabilityState set the value of the property OperabilityState
func (device *Device) SetOperabilityState(state OperabilityState) {
	if device.properties == nil {
		return
	}

	oldVariant, err := device.properties.Get(dbusDeviceInterface, propertyOperabilityState)

	if err != nil {
		return
	}

	oldState := oldVariant.Value().(OperabilityState)
	if oldState == state {
		return
	}

	device.log.Info("OperabilityState of the device", device.DevID, "changed from", oldState, "to", state)
	device.properties.SetMust(dbusDeviceInterface, propertyOperabilityState, state)
}

// SetPairingState set the value of the property PairingState
func (device *Device) SetPairingState(state PairingState) {
	if device.properties == nil {
		return
	}

	oldVariant, err := device.properties.Get(dbusDeviceInterface, propertyPairingState)

	if err != nil {
		return
	}

	oldState := oldVariant.Value().(PairingState)
	if oldState == state {
		return
	}

	device.log.Info("propertyPairingState of the device", device.DevID, "changed from", oldState, "to", state)
	device.properties.SetMust(dbusDeviceInterface, propertyPairingState, state)
}

// SetReachabilityState set the value of the property ReachabilityState
func (device *Device) SetReachabilityState(state ReachabilityState) {
	if device.properties == nil {
		return
	}

	if device.ReachabilityTimeout != 0 && state == ReachabilityOk {
		if device.timer == nil {
			device.timer = time.AfterFunc(device.ReachabilityTimeout, device.reachabilityCBTimeout)
		} else {
			device.timer.Reset(device.ReachabilityTimeout)
		}
	}

	oldVariant, err := device.properties.Get(dbusDeviceInterface, propertyReachabilityState)

	if err != nil {
		return
	}

	oldState := oldVariant.Value().(ReachabilityState)
	if oldState == state {
		return
	}

	device.log.Info("propertyReachabilityState of the device", device.DevID, "changed from", oldState, "to", state)
	device.properties.SetMust(dbusDeviceInterface, propertyReachabilityState, state)
}

// SetVersion set the value of the property Version
func (device *Device) SetVersion(newVersion string) {
	if device.properties == nil {
		return
	}

	if device.FirmwareVersion == newVersion {
		return
	}

	device.log.Info("Version of the device", device.DevID, "changed from", device.FirmwareVersion, "to", newVersion)
	device.properties.SetMust(dbusDeviceInterface, propertyVersion, newVersion)
}

// SetOption set the value of the property Option
func (device *Device) SetOption(options []byte) {
	if device.properties == nil {
		return
	}

	oldVariant, err := device.properties.Get(dbusDeviceInterface, propertyOptions)

	if err != nil {
		return
	}

	oldState := oldVariant.Value().([]byte)
	newState := []byte(options)
	if bytes.Equal(oldState, newState) {
		return
	}

	device.log.Info("propertyOptions of the device", device.DevID, "changed from", oldState, "to", newState)
	device.properties.SetMust(dbusDeviceInterface, propertyOptions, newState)
}
