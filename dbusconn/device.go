package dbusconn

import (
	"sync"

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

// Device sent over dbus
type Device struct {
	sync.Mutex

	protocol *Protocol

	DevID           string
	Address         string
	TypeID          string
	TypeVersion     string
	Options         map[string]string
	FirmwareVersion string

	properties *prop.Properties
	Items      map[string]*Item

	log *logging.Logger

	SetDeviceOptionCb setDeviceOptionInterface
	UpdateFirmwareCb  updateFirmwareInterface
}

// OperabilityState informs if the device work
type OperabilityState string

// PairingState informs if the state of the pairing
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

func (d *Device) UpdateFirmware(data string) (string, *dbus.Error) {
	if !isNil(d.UpdateFirmwareCb) {
		return d.UpdateFirmwareCb.UpdateFirmware(d, data)
	}
	d.log.Warning("Update firmware not implemented")
	return "", nil
}

func initDevice(devID string, address string, typeID string, typeVersion string, options map[string]string, p *Protocol) *Device {
	return &Device{
		DevID:       devID,
		Address:     address,
		TypeID:      typeID,
		TypeVersion: typeVersion,
		Options:     options,
		Items:       make(map[string]*Item),
		protocol:    p,
		log:         p.log,
	}
}

// EmitDeviceAdded to call when a device is added
func (dc *Dbus) emitDeviceAdded(device *Device) {
	args := make([]interface{}, 4)
	args[0] = device.Address
	args[1] = device.TypeID
	args[2] = device.TypeVersion
	args[3] = device.Options
	path := dbus.ObjectPath(dbusPathPrefix + dc.ProtocolName + "/" + device.DevID)
	dc.conn.Emit(path, dbusDeviceInterface+"."+signalDeviceAdded, args...)
}

// EmitDeviceAdded to call when a device is added
func (dc *Dbus) emitDeviceRemoved(devID string) {
	path := dbus.ObjectPath(dbusPathPrefix + dc.ProtocolName + "/" + devID)
	dc.conn.Emit(path, dbusDeviceInterface+"."+signalDeviceRemoved)
}

// ExportDeviceOnDbus export a device on dbus
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

// AddItem called to add a new item to this device
func (device *Device) AddItem(itemID string, typeID string, typeVersion string, options map[string]string) (bool, *dbus.Error) {
	device.log.Info("AddItem called - itemID:", itemID, "typeID:", typeID, "typeVersion:", typeVersion, "options:", options)

	device.Lock()
	_, itemPresent := device.Items[itemID]
	if !itemPresent {
		item := InitItem(itemID, typeID, typeVersion, options, device)
		device.Items[itemID] = item
		device.protocol.dc.ExportItemOnDbus(device.DevID, item)

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

// RemoveItem called to remove an item to this device
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
				Value:    string(OperabilityUnknown),
				Writable: false,
				Emit:     prop.EmitTrue,
				Callback: nil,
			},
			propertyPairingState: {
				Value:    string(PairingUnknown),
				Writable: false,
				Emit:     prop.EmitTrue,
				Callback: nil,
			},
			propertyReachabilityState: {
				Value:    string(ReachabilityKo),
				Writable: false,
				Emit:     prop.EmitTrue,
				Callback: nil,
			},
			propertyVersion: {
				Value:    string(""),
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

	oldState := oldVariant.Value().(string)
	newState := string(state)
	if oldState == newState {
		return
	}

	device.log.Info("OperabilityState of the device", device.DevID, "changed from", oldState, "to", newState)
	device.properties.SetMust(dbusDeviceInterface, propertyOperabilityState, newState)
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

	oldState := oldVariant.Value().(string)
	newState := string(state)
	if oldState == newState {
		return
	}

	device.log.Info("propertyPairingState of the device", device.DevID, "changed from", oldState, "to", newState)
	device.properties.SetMust(dbusDeviceInterface, propertyPairingState, newState)
}

// SetReachabilityState set the value of the property ReachabilityState
func (device *Device) SetReachabilityState(state ReachabilityState) {
	if device.properties == nil {
		return
	}

	oldVariant, err := device.properties.Get(dbusDeviceInterface, propertyReachabilityState)

	if err != nil {
		return
	}

	oldState := oldVariant.Value().(string)
	newState := string(state)
	if oldState == newState {
		return
	}

	device.log.Info("propertyReachabilityState of the device", device.DevID, "changed from", oldState, "to", newState)
	device.properties.SetMust(dbusDeviceInterface, propertyReachabilityState, newState)
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
	device.FirmwareVersion = newVersion
	device.properties.SetMust(dbusDeviceInterface, propertyVersion, newVersion)
}

// SetOption set the value of the property Option
func (device *Device) SetOption(key string, newValue string) {
	oldVal := "empty"
	if device.properties == nil {
		return
	}

	if val, ok := device.Options[key]; ok {
		if val == newValue {
			return
		}
		oldVal = val
	}

	device.log.Info("Option", key, "of the device", device.DevID, "changed from", oldVal, "to", newValue)
	device.Options[key] = newValue
	device.properties.SetMust(dbusDeviceInterface, propertyOptions, device.Options)
}
