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

	DevID              string
	Address            string
	TypeID             string
	TypeVersion        string
	Options            []byte
	FirmwareVersion    string
	Operability        OperabilityState
	PairingState       PairingState
	OperabilityTimeout time.Duration

	Items map[string]*Item

	dc         *Dbus
	timer      *time.Timer
	properties *prop.Properties
	log        *logging.Logger

	addItemCB            interface{ AddItem(*Item) }
	removeItemCB         interface{ RemoveItem(string, string) }
	setDeviceOptionCb    interface{ SetDeviceOptions(*Device) }
	updateFirmwareCb     interface{ UpdateFirmware(*Device, string) }
	operabilityTimeoutCB interface{ OperabilityWentKo(*Device) }
}

// OperabilityState informs if the device work
type OperabilityState string

// PairingState informs the state of the pairing
type PairingState string

func initDevice(devID string, address string, typeID string, typeVersion string, options []byte, p *Protocol) {
	d := &Device{
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
		dc:           p.dc,
	}
	p.Devices[devID] = d

	path := dbus.ObjectPath(dbusPathPrefix + d.Protocol.protocolName + "/" + d.DevID)

	d.SetDbusProperties(nil)
	d.SetDbusMethods(nil)
	d.SetCallbacks(d.Protocol.cbs)
	if !isNil(p.addDeviceCB) {
		go p.addDeviceCB.AddDevice(p.Devices[devID])
	}

	//Emit Device Added
	p.dc.conn.Emit(path, dbusDeviceInterface+"."+signalDeviceAdded, []interface{}{d.Address, d.TypeID, d.TypeVersion, d.Options})
}

func removeDevice(d *Device) {
	p := d.Protocol
	path := dbus.ObjectPath(dbusPathPrefix + p.protocolName + "/" + d.DevID)
	d.Lock()
	for _, i := range d.Items {
		removeItem(i)
	}
	if !isNil(p.removeDeviceCB) {
		go p.removeDeviceCB.RemoveDevice(d.DevID)
	}
	d.Unlock()
	delete(p.Devices, d.DevID)
	p.dc.conn.Emit(path, dbusDeviceInterface+"."+signalDeviceRemoved)
	p.dc.conn.Export(nil, path, dbusDeviceInterface)
}

func (d *Device) operabilityCBTimeout() {
	d.SetOperabilityState(OperabilityKo)

	if !isNil(d.operabilityTimeoutCB) {
		go d.operabilityTimeoutCB.OperabilityWentKo(d)
	}
}

func (d *Device) setDeviceOptions(c *prop.Change) *dbus.Error {
	if !isNil(d.setDeviceOptionCb) {
		go d.setDeviceOptionCb.SetDeviceOptions(d)
	} else {
		d.log.Warning("No Options")
	}
	return nil
}

// AddItem adds a new item to device
func (d *Device) AddItem(itemID string, typeID string, typeVersion string, options []byte) (bool, *dbus.Error) {
	d.log.Info("AddItem called - itemID:", itemID, "typeID:", typeID, "typeVersion:", typeVersion, "options:", options)
	d.Lock()
	_, itemPresent := d.Items[itemID]
	if !itemPresent {
		initItem(itemID, typeID, typeVersion, options, d)
		d.Unlock()
		return false, nil
	}
	d.Unlock()
	return true, nil
}

// RemoveItem remove item from device
func (d *Device) RemoveItem(itemID string) *dbus.Error {
	d.log.Info("RemoveItem called - itemID:", itemID)
	d.Lock()
	i, present := d.Items[itemID]
	if present {
		removeItem(i)
	}
	d.Unlock()
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

// SetOperabilityState set the value of the property OperabilityState
func (d *Device) SetOperabilityState(state OperabilityState) {
	if d.properties == nil {
		return
	}

	if d.OperabilityTimeout != 0 && state == OperabilityOk {
		if d.timer == nil {
			d.timer = time.AfterFunc(d.OperabilityTimeout, d.operabilityCBTimeout)
		} else {
			d.timer.Reset(d.OperabilityTimeout)
		}
	}

	oldVariant, err := d.properties.Get(dbusDeviceInterface, propertyOperabilityState)

	if err != nil {
		return
	}

	oldState := oldVariant.Value().(OperabilityState)
	if oldState == state {
		return
	}

	d.log.Info("OperabilityState of the device", d.DevID, "changed from", oldState, "to", state)
	d.properties.SetMust(dbusDeviceInterface, propertyOperabilityState, state)
}

// SetPairingState set the value of the property PairingState
func (d *Device) SetPairingState(state PairingState) {
	if d.properties == nil {
		return
	}

	oldVariant, err := d.properties.Get(dbusDeviceInterface, propertyPairingState)

	if err != nil {
		return
	}

	oldState := oldVariant.Value().(PairingState)
	if oldState == state {
		return
	}

	d.log.Info("propertyPairingState of the device", d.DevID, "changed from", oldState, "to", state)
	d.properties.SetMust(dbusDeviceInterface, propertyPairingState, state)
}

// SetVersion set the value of the property Version
func (d *Device) SetVersion(newVersion string) {
	if d.properties == nil {
		return
	}

	if d.FirmwareVersion == newVersion {
		return
	}

	d.log.Info("Version of the device", d.DevID, "changed from", d.FirmwareVersion, "to", newVersion)
	d.properties.SetMust(dbusDeviceInterface, propertyVersion, newVersion)
}

// SetOption set the value of the property Option
func (d *Device) SetOption(options []byte) {
	if d.properties == nil {
		return
	}

	oldVariant, err := d.properties.Get(dbusDeviceInterface, propertyOptions)

	if err != nil {
		return
	}

	oldState := oldVariant.Value().([]byte)
	newState := []byte(options)
	if bytes.Equal(oldState, newState) {
		return
	}

	d.log.Info("propertyOptions of the device", d.DevID, "changed from", oldState, "to", newState)
	d.properties.SetMust(dbusDeviceInterface, propertyOptions, newState)
}

// SetCallbacks set new callbacks for this device
func (d *Device) SetCallbacks(cbs interface{}) {
	switch cb := cbs.(type) {
	case interface{ AddItem(*Item) }:
		d.addItemCB = cb
	}
	switch cb := cbs.(type) {
	case interface{ RemoveItem(string, string) }:
		d.removeItemCB = cb
	}
	switch cb := cbs.(type) {
	case interface{ SetDeviceOptions(*Device) }:
		d.setDeviceOptionCb = cb
	}
	switch cb := cbs.(type) {
	case interface{ UpdateFirmware(*Device, string) }:
		d.updateFirmwareCb = cb
	}
	switch cb := cbs.(type) {
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

// SetDbusProperties set new DBus properties for this device
func (d *Device) SetDbusProperties(externalProperties map[string]*prop.Prop) bool {
	path := dbus.ObjectPath(dbusPathPrefix + d.Protocol.protocolName + "/" + d.DevID)
	propsSpec := map[string]map[string]*prop.Prop{
		dbusDeviceInterface: {
			propertyOperabilityState: {
				Value:    d.Operability,
				Writable: false,
				Emit:     prop.EmitTrue,
				Callback: nil,
			},
			propertyPairingState: {
				Value:    d.PairingState,
				Writable: false,
				Emit:     prop.EmitTrue,
				Callback: nil,
			},
			propertyVersion: {
				Value:    d.FirmwareVersion,
				Writable: false,
				Emit:     prop.EmitTrue,
				Callback: nil,
			},
			propertyOptions: {
				Value:    d.Options,
				Writable: true,
				Emit:     prop.EmitTrue,
				Callback: d.setDeviceOptions,
			},
		},
	}

	for pName, p := range externalProperties {
		propsSpec[dbusDeviceInterface][pName] = p
	}

	properties, err := prop.Export(d.dc.conn, path, propsSpec)
	if err == nil {
		d.properties = properties
	} else {
		d.log.Error("Fail to export the properties of the device", d.DevID, err)
		return false
	}
	return true
}
