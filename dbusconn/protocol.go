package dbusconn

import (
	"sync"

	"github.com/godbus/dbus/v5"
	"github.com/godbus/dbus/v5/prop"
	"github.com/op/go-logging"
)

const (
	propertyLogLevel = "LogLevel"
)

// ProtocolInterface callback called from Protocol Dbus Methods
type ProtocolInterface interface {
	AddDevice(*Device)
	RemoveDevice(string)
	AddItem(*Item)
	RemoveItem(string, string)
}

// Protocol is a dbus object which represents the states of the module
type Protocol struct {
	Callbacks  ProtocolInterface
	dc         *Dbus
	Devices    map[string]*Device
	ready      bool
	properties *prop.Properties
	log        *logging.Logger
	sync.Mutex
}

// ExportProtocolObject Initializes and exports the Protocol object on DBus
func (dc *Dbus) ExportProtocolObject(protocol string) (*Protocol, bool) {
	if dc.conn == nil {
		dc.Log.Warning("Unable to export Protocol dbus object because dbus connection nil")
		return nil, false
	}

	var proto = &Protocol{ready: false, dc: dc, Devices: make(map[string]*Device), log: dc.Log}
	path := dbus.ObjectPath(dbusPathPrefix + protocol)

	// properties
	propsSpec := initProtocolProp(proto)
	properties, err := prop.Export(dc.conn, path, propsSpec)
	if err == nil {
		proto.properties = properties
	} else {
		proto.log.Error("Fail to export the properties of the protocol", proto, err)
	}

	err = dc.conn.Export(proto, path, dbusProtocolInterface)
	if err != nil {
		proto.log.Warning("Fail to export Module dbus object", err)
		return nil, false
	}

	return proto, true
}

func (p *Protocol) setReady() {
	p.Lock()
	p.ready = true
	p.Unlock()
}

// IsReady dbus method to known is the protocol is ready or not
func (p *Protocol) IsReady() (bool, *dbus.Error) {
	p.Lock()
	var ready = p.ready
	p.Unlock()

	return ready, nil
}

func (p *Protocol) AddDevice(devID string, comID string, typeID string, typeVersion string, options map[string]string) (bool, *dbus.Error) {
	p.Lock()
	_, alreadyAdded := p.Devices[devID]
	if !alreadyAdded {
		device := initDevice(devID, comID, typeID, typeVersion, options, p)
		p.Devices[devID] = device
		p.dc.exportDeviceOnDbus(p.Devices[devID])
		if !isNil(p.Callbacks) {
			go p.Callbacks.AddDevice(p.Devices[devID])
		}
		p.dc.emitDeviceAdded(device)
	}
	p.Unlock()
	return alreadyAdded, nil
}

func (p *Protocol) RemoveDevice(devID string) *dbus.Error {
	p.Lock()
	device, devicePresent := p.Devices[devID]

	if !devicePresent {
		p.Unlock()
		return nil
	}
	device.Lock()

	for item := range device.Items {
		delete(device.Items, item)
	}
	if !isNil(p.Callbacks) {
		go p.Callbacks.RemoveDevice(devID)
	}
	device.Unlock()
	delete(p.Devices, devID)
	p.dc.emitDeviceRemoved(devID)
	p.Unlock()
	return nil
}

func (p *Protocol) setLogLevel(c *prop.Change) *dbus.Error {
	loglevel := c.Value.(string)
	level, err := logging.LogLevel(loglevel)
	if err == nil {
		logging.SetLevel(level, p.dc.Log.Module)
		p.log.Info("Log level has been set to ", c.Value.(string))
		return &dbus.ErrMsgInvalidArg
	} else {
		p.log.Error(err)
	}
	return nil
}

func initProtocolProp(p *Protocol) map[string]map[string]*prop.Prop {
	return map[string]map[string]*prop.Prop{
		dbusProtocolInterface: {
			propertyLogLevel: {
				Value:    logging.GetLevel(p.log.Module).String(),
				Writable: true,
				Emit:     prop.EmitTrue,
				Callback: p.setLogLevel,
			},
		},
	}
}
