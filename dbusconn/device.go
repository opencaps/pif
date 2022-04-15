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

	propertyOperabilityState = "OperabilityState"
	propertyPairingState     = "PairingState"
	propertyVersion          = "Version"
	propertyOptions          = "Options"

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
)

// Device object structure
type Device struct {
	sync.Mutex

	Protocol *Protocol

	DevID           string
	Address         string
	TypeID          string
	TypeVersion     string
	Options         []byte
	FirmwareVersion string
	Operability     OperabilityState
	PairingState    PairingState

	OperabilityTimeout time.Duration

	timer *time.Timer

	properties *prop.Properties
	Items      map[string]*Item

	log *logging.Logger

	setDeviceOptionCb    interface{ SetDeviceOptions(*Device) }
	updateFirmwareCb     interface{ UpdateFirmware(*Device, string) }
	operabilityTimeoutCB interface{ OperabilityWentKo(*Device) }
}

// OperabilityState informs if the device work
type OperabilityState string

// PairingState informs the state of the pairing
type PairingState string

func (d *Device) setDeviceOptions(c *prop.Change) *dbus.Error {
	if !isNil(d.setDeviceOptionCb) {
		go d.setDeviceOptionCb.SetDeviceOptions(d)
	} else {
		d.log.Warning("No Options")
	}
	return nil
}

//UpdateFirmware is the dbus method to update the firmware of the device
func (d *Device) UpdateFirmware(data string) (string, *dbus.Error) {
	if !isNil(d.updateFirmwareCb) {
		go d.updateFirmwareCb.UpdateFirmware(d, data)
	}
	d.log.Warning("Update firmware not implemented")
	return "", nil
}

func (d *Device) operabilityCBTimeout() {
	d.SetOperabilityState(OperabilityKo)

	if !isNil(d.operabilityTimeoutCB) {
		go d.operabilityTimeoutCB.OperabilityWentKo(d)
	}
}

func initDevice(devID string, address string, typeID string, typeVersion string, options []byte, p *Protocol) *Device {
	return &Device{
		DevID:        devID,
		Address:      address,
		TypeID:       typeID,
		TypeVersion:  typeVersion,
		Options:      options,
		PairingState: PairingUnknown,
		Operability:  OperabilityUnknown,
		Items:        make(map[string]*Item),
		Protocol:     p,
		log:          p.log,
	}
}

func (dc *Dbus) emitDeviceAdded(device *Device) {
	args := make([]interface{}, 4)
	args[0] = device.Address
	args[1] = device.TypeID
	args[2] = device.TypeVersion
	args[3] = device.Options
	path := dbus.ObjectPath(dbusPathPrefix + device.Protocol.protocolName + "/" + device.DevID)
	dc.conn.Emit(path, dbusDeviceInterface+"."+signalDeviceAdded, args...)
}

func (dc *Dbus) emitDeviceRemoved(device *Device) {
	path := dbus.ObjectPath(dbusPathPrefix + device.Protocol.protocolName + "/" + device.DevID)
	dc.conn.Emit(path, dbusDeviceInterface+"."+signalDeviceRemoved)
}

func (dc *Dbus) exportDeviceOnDbus(device *Device) {
	if dc.conn == nil {
		dc.Log.Warning("Unable to export dbus object because dbus connection nil")
	}

	path := dbus.ObjectPath(dbusPathPrefix + device.Protocol.protocolName + "/" + device.DevID)

	// properties
	propsSpec := initDeviceProp(device)
	properties, err := prop.Export(dc.conn, path, propsSpec)
	if err == nil {
		device.properties = properties
	} else {
		dc.Log.Error("Fail to export the properties of the device", device.DevID, err)
	}

	// object
	device.SetDbusMethods(nil)

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
		device.Protocol.dc.exportItemOnDbus(item)
		item.SetCallbacks(device.Protocol.cbs)

		if !isNil(device.Protocol.addItemCB) {
			go device.Protocol.addItemCB.AddItem(item)
		}
		device.Protocol.dc.emitItemAdded(item)
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
	item, present := device.Items[itemID]
	if present {
		device.Protocol.dc.emitItemRemoved(item)
		delete(device.Items, itemID)
		if !isNil(device.Protocol.removeItemCB) {
			go device.Protocol.removeItemCB.RemoveItem(device.DevID, itemID)
		}
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

	if device.OperabilityTimeout != 0 && state == OperabilityOk {
		if device.timer == nil {
			device.timer = time.AfterFunc(device.OperabilityTimeout, device.operabilityCBTimeout)
		} else {
			device.timer.Reset(device.OperabilityTimeout)
		}
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

// SetCallbacks set new callbacks for this device
func (d *Device) SetCallbacks() {
	switch cb := d.Protocol.cbs.(type) {
	case interface{ SetDeviceOptions(*Device) }:
		d.setDeviceOptionCb = cb
	}
	switch cb := d.Protocol.cbs.(type) {
	case interface{ UpdateFirmware(*Device, string) }:
		d.updateFirmwareCb = cb
	}
	switch cb := d.Protocol.cbs.(type) {
	case interface{ OperabilityWentKo(*Device) }:
		d.operabilityTimeoutCB = cb
	}
}

// SetDbusMethods set new dbusMethods for this device
func (d *Device) SetDbusMethods(externalMethods map[string]interface{}) bool {
	path := dbus.ObjectPath(dbusPathPrefix + d.Protocol.protocolName + "/" + d.DevID)
	exportedMethods := make(map[string]interface{})
	exportedMethods["AddItem"] = d.AddItem
	exportedMethods["RemoveItem"] = d.RemoveItem

	for name, inter := range externalMethods {
		exportedMethods[name] = inter
	}

	err := d.Protocol.dc.conn.ExportMethodTable(exportedMethods, path, dbusDeviceInterface)
	if err != nil {
		d.log.Warning("Fail to export device dbus object", d.DevID, err)
		return false
	}
	return true
}
