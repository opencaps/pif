package dbusconn

import (
	"strings"
	"sync"
	"time"

	"github.com/godbus/dbus"
	"github.com/godbus/dbus/prop"
)

const (
	msgBodyNotValid    = "body not valid"
	signalAddDevice    = "AddDevice"
	signalDeviceAdded  = "DeviceAdded"
	signalRemoveDevice = "RemoveDevice"

	propertyOperabilityState  = "OperabilityState"
	propertyPairingState      = "PairingState"
	propertyReachabilityState = "ReachabilityState"
	propertyVersion           = "Version"

	// OperabilityOk state 'ok' for OperabilityState
	OperabilityOk OperabilityState = 0
	// OperabilityPartial state 'partial' for OperabilityState
	OperabilityPartial OperabilityState = 1
	// OperabilityKo state 'Ko' for OperabilityState
	OperabilityKo OperabilityState = 2
	// OperabilityUnknown state 'unknown' for OperabilityState
	OperabilityUnknown OperabilityState = 3

	// PairingOk state 'ok' for PairingState
	PairingOk PairingState = 0
	// PairingInProgres state 'in progres' for PairingState
	PairingInProgres PairingState = 1
	// PairingKo state 'ko' for PairingState
	PairingKo PairingState = 2
	// PairingUnknown state 'unknown' for PairingState
	PairingUnknown PairingState = 3
	// PairingNotNeeded state 'not needed' for PairingState
	PairingNotNeeded PairingState = 4

	// ReachabilityOk state 'ok' for ReachabilityState
	ReachabilityOk ReachabilityState = 0
	// ReachabilityKo state 'ko' for ReachabilityState
	ReachabilityKo ReachabilityState = 1
	// ReachabilityRescue state 'rescue' for ReachabilityState
	ReachabilityRescue ReachabilityState = 2
	// ReachabilityUnknown state 'unknown' for ReachabilityState
	ReachabilityUnknown ReachabilityState = 3

	frequencyMaxAttempts = 3
)

// DeviceInterface callback called from device dbus events
type DeviceInterface interface {
	FindDriverFrequency(string, string) (*int, bool)
	SetItem(*Item, []byte) bool
	SetOptionsItem(*Item) bool
	SetOptionsDevice(*Device) bool
	AddItem(*Device, *Item)
	RemoveItem(*Device, *Item)
	SetUpdateMode(*Device, int) bool
}

// Device sent over dbus
type Device struct {
	Lock sync.Mutex

	DevID       string
	Address     string
	TypeID      string
	TypeVersion string
	Options     map[string]string

	properties *prop.Properties
	Items      map[string]*Item

	frequency          *int // in ms
	lastReachabilityOk *time.Time

	callbacks DeviceInterface
}

// OperabilityState informs if the device work
type OperabilityState int32

// PairingState informs if the state of the pairing
type PairingState int32

// ReachabilityState informs if the device is reachable
type ReachabilityState int32

// InitDevice to init a Device struct
func InitDevice(devID string, address string, typeID string, typeVersion string, options map[string]string, deviceInterface DeviceInterface) *Device {
	return &Device{
		DevID:       devID,
		Address:     address,
		TypeID:      typeID,
		TypeVersion: typeVersion,
		Options:     options,
		Items:       make(map[string]*Item),
		callbacks:   deviceInterface,
	}
}

// EmitDeviceAdded to call when a device is added
func (dc *Dbus) EmitDeviceAdded(devID string, alreadyAdded bool) {
	path := dbus.ObjectPath(dbusPathPrefix + dc.Protocol + "/" + devID)
	dc.conn.Emit(path, dbusInterface+"."+signalDeviceAdded, alreadyAdded)
}

// ExportDeviceOnDbus export a device on dbus
func (dc *Dbus) ExportDeviceOnDbus(device *Device) {
	if dc.conn == nil {
		log.Warning("Unable to export dbus object because dbus connection nil")
	}

	path := dbus.ObjectPath(dbusPathPrefix + dc.Protocol + "/" + device.DevID)

	// properties
	propsSpec := initProp(device.dbusInterface())
	properties, err := prop.Export(dc.conn, path, propsSpec)
	if err == nil {
		device.properties = properties
	} else {
		log.Error("Fail to export the properties of the device", device.DevID, err)
	}

	// object
	dc.conn.Export(device, path, dbusInterface)

	log.Info("Device exported:", path)
}

func (dc *Dbus) handleSignalAddDevice(signal *dbus.Signal) {
	if len(signal.Body) < 4 {
		log.Warning("Signal", signalAddDevice, msgBodyNotValid, signal.Body)
		return
	}

	devID, conv1 := signal.Body[0].(string)
	address, conv2 := signal.Body[1].(string)
	typeID, conv3 := signal.Body[2].(string)
	typeVersion, conv4 := signal.Body[3].(string)
	options, conv5 := signal.Body[4].(map[string]string)

	if !conv1 || !conv2 || !conv3 || !conv4 || !conv5 {
		log.Warning("Signal", signalAddDevice, msgBodyNotValid, signal.Body)
		return
	}
	log.Info("Signal", signalAddDevice, "received - devID:", devID, "address:", address, "typeID:", typeID, "typeVersion:", typeVersion, "options:", options)
	dc.Callbacks.AddDevice(devID, strings.ToUpper(address), typeID, typeVersion, options)
}

// SetOptions called when a new options come from Hemis
func (device *Device) SetOptions(options map[string]string) (bool, *dbus.Error) {
	device.Options = options
	return device.callbacks.SetOptionsDevice(device), nil
}

func (dc *Dbus) handleSignalRemoveDevice(signal *dbus.Signal) {
	path := strings.Split(string(signal.Path), "/")
	len := len(path)
	if len < 1 {
		log.Warning("Signal", signalRemoveDevice, "path not valid", signal.Path)
		return
	}
	devID := path[len-1]
	log.Info("Signal", signalRemoveDevice, "received - devID:", devID)
	dc.Callbacks.RemoveDevice(devID)
}

// AddItem called to add a new item to this device
func (device *Device) AddItem(itemID string, typeID string, typeVersion string, options map[string]string) (bool, *dbus.Error) {
	log.Info("AddItem called - itemID:", itemID, "typeID:", typeID, "typeVersion:", typeVersion, "options:", options)

	device.Lock.Lock()
	item, itemPresent := device.Items[itemID]
	if !itemPresent {
		frequency, found := device.callbacks.FindDriverFrequency(typeID, typeVersion)

		if !found {
			log.Warning("Unable to add the item because driver not found", itemID)
			device.Lock.Unlock()
			return false, nil
		}

		item = InitItem(itemID, typeID, typeVersion, device.Address, options, device.callbacks)
		device.Items[itemID] = item

		if frequency != nil && device.frequency == nil {
			device.frequency = frequency
		}
	}
	device.Lock.Unlock()

	if !itemPresent {
		device.callbacks.AddItem(device, item)
	}

	return true, nil
}

// RemoveItem called to remove an item to this device
func (device *Device) RemoveItem(itemID string) (bool, *dbus.Error) {
	log.Info("RemoveItem called - itemID:", itemID)

	device.Lock.Lock()
	item, present := device.Items[itemID]
	if present {
		delete(device.Items, itemID)
	} else {
		log.Warning("Fail to remove the item", itemID)
	}
	device.Lock.Unlock()

	if present {
		device.callbacks.RemoveItem(device, item)
	}

	return present, nil
}

func (device *Device) dbusInterface() string {
	return dbusInterface + "." + device.DevID
}

func initProp(dbusInterface string) map[string]map[string]*prop.Prop {
	return map[string]map[string]*prop.Prop{
		dbusInterface: {
			propertyOperabilityState: {
				Value:    int32(OperabilityUnknown),
				Writable: false,
				Emit:     prop.EmitTrue,
				Callback: nil,
			},
			propertyPairingState: {
				Value:    int32(PairingUnknown),
				Writable: false,
				Emit:     prop.EmitTrue,
				Callback: nil,
			},
			propertyReachabilityState: {
				Value:    int32(ReachabilityKo),
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
		},
	}
}

// SetOperabilityState set the value of the property OperabilityState
func (device *Device) SetOperabilityState(state OperabilityState) {
	if device.properties == nil {
		return
	}

	dbusInterface := device.dbusInterface()
	oldVariant, err := device.properties.Get(dbusInterface, propertyOperabilityState)

	if err != nil {
		return
	}

	oldState := oldVariant.Value().(int32)
	newState := int32(state)
	if oldState == newState {
		return
	}

	log.Info("OperabilityState of the device", device.DevID, "changed from", oldState, "to", newState)
	device.properties.SetMust(dbusInterface, propertyOperabilityState, newState)
}

// SetPairingState set the value of the property PairingState
func (device *Device) SetPairingState(state PairingState) {
	if device.properties == nil {
		return
	}

	dbusInterface := device.dbusInterface()
	oldVariant, err := device.properties.Get(dbusInterface, propertyPairingState)

	if err != nil {
		return
	}

	oldState := oldVariant.Value().(int32)
	newState := int32(state)
	if oldState == newState {
		return
	}

	log.Info("propertyPairingState of the device", device.DevID, "changed from", oldState, "to", newState)
	device.properties.SetMust(dbusInterface, propertyPairingState, newState)
}

// HeartBeat return true if the hearbeat of the device is ok
func (device *Device) HeartBeat() bool {
	if device.frequency == nil || device.lastReachabilityOk == nil {
		return false
	}

	timeMin := time.Now().Add(-time.Duration((*device.frequency)*frequencyMaxAttempts) * time.Millisecond)
	return device.lastReachabilityOk.After(timeMin)
}

// SetReachabilityState set the value of the property ReachabilityState
func (device *Device) SetReachabilityState(state ReachabilityState) {
	if state == ReachabilityOk {
		now := time.Now()
		device.lastReachabilityOk = &now
	}

	if device.properties == nil {
		return
	}

	dbusInterface := device.dbusInterface()
	oldVariant, err := device.properties.Get(dbusInterface, propertyReachabilityState)

	if err != nil {
		return
	}

	oldState := oldVariant.Value().(int32)
	newState := int32(state)
	if oldState == newState {
		return
	}

	log.Info("propertyReachabilityState of the device", device.DevID, "changed from", oldState, "to", newState)
	device.properties.SetMust(dbusInterface, propertyReachabilityState, newState)
}

// SetVersion set the value of the property Version
func (device *Device) SetVersion(newVersion string) {
	if device.properties == nil {
		return
	}

	dbusInterface := device.dbusInterface()
	oldVariant, err := device.properties.Get(dbusInterface, propertyVersion)

	if err != nil {
		return
	}

	oldState := oldVariant.Value().(string)
	if oldState == newVersion {
		return
	}

	log.Info("Version of the device", device.DevID, "changed from", oldState, "to", newVersion)
	device.properties.SetMust(dbusInterface, propertyVersion, newVersion)
}

// SetUpdateMode set the update mode
func (device *Device) SetUpdateMode(updateMode int) *dbus.Error {
	if device.callbacks.SetUpdateMode(device, updateMode) {
		return nil
	}
	return dbus.NewError("Failed", nil)
}
