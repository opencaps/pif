package dbusconn

import (
	"sync"

	"github.com/godbus/dbus/v5"
	"github.com/godbus/dbus/v5/prop"
	"github.com/op/go-logging"
)

const (
	propertyLogLevel          = "LogLevel"
	propertyReachabilityState = "ReachabilityState"

	signalBridgeAdded   = "BridgeAdded"
	signalBridgeRemoved = "BridgeRemoved"

	// ReachabilityOk state 'ok' for ReachabilityState
	ReachabilityOk ReachabilityState = "OK"
	// ReachabilityKo state 'ko' for ReachabilityState
	ReachabilityKo ReachabilityState = "KO"
	// ReachabilityUnknown state 'unknown' for ReachabilityState
	ReachabilityUnknown ReachabilityState = "UNKNOWN"
)

// ReachabilityState informs if the device is reachable
type ReachabilityState string

// ProtocolInterface callback called from Protocol Dbus Methods
type ProtocolInterface interface {
	AddDevice(*Device)
	RemoveDevice(string)
	AddItem(*Item)
	RemoveItem(string, string)
}

// ProtocolInterface callback called from Protocol Dbus Methods
type BridgeInterface interface {
	AddBridge(*Protocol)
	RemoveBridge(string)
}

// Protocol is a dbus object which represents the states of a protocol
type Protocol struct {
	Callbacks    ProtocolInterface
	dc           *Dbus
	Devices      map[string]*Device
	ready        bool
	log          *logging.Logger
	properties   *prop.Properties
	Reachability ReachabilityState
	protocolName string
	sync.Mutex
}

// RootProtocol is a dbus object which represents the states of the root protocol
type RootProto struct {
	Protocol   *Protocol
	Callbacks  BridgeInterface
	dc         *Dbus
	properties *prop.Properties
	log        *logging.Logger
}

// Protocol is a dbus object which represents the states of a bridge protocol
type BridgeProto struct {
	Protocol *Protocol
	dc       *Dbus
}

func (dc *Dbus) newProtocol(name string) (*Protocol, bool) {
	var proto = &Protocol{ready: false, dc: dc, Devices: make(map[string]*Device), log: dc.Log, protocolName: name, Reachability: ReachabilityUnknown}
	path := dbus.ObjectPath(dbusPathPrefix + name)

	propsSpec := initProtocolProp(proto)
	properties, err := prop.Export(dc.conn, path, propsSpec)

	if err == nil {
		proto.properties = properties
	} else {
		proto.log.Error("Fail to export the properties of the protocol", name, err)
		return nil, false
	}

	err = dc.conn.Export(proto, path, dbusProtocolInterface)
	if err != nil {
		proto.log.Warning("Fail to export Module dbus object", err)
		return nil, false
	}

	return proto, true
}

func (dc *Dbus) exportRootProtocolObject(protocol string) (*Protocol, bool) {
	if dc.conn == nil {
		dc.Log.Warning("Unable to export Protocol dbus object because dbus connection nil")
		return nil, false
	}

	proto, ok := dc.newProtocol(protocol)
	if !ok {
		return nil, false
	}

	path := dbus.ObjectPath(dbusPathPrefix + protocol)

	// properties
	rootPropsSpec := initRootProtocolProp(&dc.RootProtocol)
	rootProperties, err := prop.Export(dc.conn, path, rootPropsSpec)
	if err == nil {
		dc.RootProtocol.properties = rootProperties
	} else {
		proto.log.Error("Fail to export the properties of the root protocol", protocol, err)
	}

	err = dc.conn.Export(dc.RootProtocol, path, dbusProtocolInterface)
	if err != nil {
		proto.log.Warning("Fail to export Module dbus object", err)
		return nil, false
	}

	return proto, true
}

// IsReady dbus method to know if the protocol is ready or not
func (p *Protocol) IsReady() (bool, *dbus.Error) {
	p.Lock()
	var ready = p.ready
	p.Unlock()

	return ready, nil
}

func (dc *Dbus) emitBridgeAdded(bridgeID string) {
	path := dbus.ObjectPath(dbusPathPrefix + dc.ProtocolName + "_" + bridgeID)
	dc.conn.Emit(path, dbusProtocolInterface+"."+signalBridgeAdded)
}

func (dc *Dbus) emitBridgeRemoved(bridgeID string) {
	path := dbus.ObjectPath(dbusPathPrefix + dc.ProtocolName + "_" + bridgeID)
	dc.conn.Emit(path, dbusProtocolInterface+"."+signalBridgeRemoved)
}

//AddDevice is the dbus method to add a new device
func (p *Protocol) AddDevice(devID string, comID string, typeID string, typeVersion string, options []byte) (bool, *dbus.Error) {
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

//RemoveDevice is the dbus method to remove a device
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

// Ready set the Protocol object parameter "ready" to true
func (p *Protocol) Ready() {
	if p != nil {
		p.Lock()
		p.ready = true
		p.Unlock()
	}
}

//AddBridge is the dbus method to add a new bridge
func (r *RootProto) AddBridge(bridgeID string) (bool, *dbus.Error) {
	protoName := r.dc.ProtocolName + "_" + bridgeID
	r.Protocol.Lock()
	_, alreadyAdded := r.dc.Bridges[bridgeID]
	if !alreadyAdded {

		proto, ok := r.dc.newProtocol(protoName)
		if !ok {
			return false, &dbus.ErrMsgNoObject
		}
		var bridge = &BridgeProto{Protocol: proto, dc: r.dc}
		r.dc.Bridges[bridgeID] = bridge
		if !isNil(r.Callbacks) {
			go r.Callbacks.AddBridge(proto)
		}
		r.dc.emitBridgeAdded(bridgeID)
	}
	r.Protocol.Unlock()
	return alreadyAdded, nil
}

//RemoveBridge is the dbus method to remove a bridge
func (r *RootProto) RemoveBridge(bridgeID string) *dbus.Error {

	r.Protocol.Lock()
	bridge, bridgePresent := r.dc.Bridges[bridgeID]

	if !bridgePresent {
		r.Protocol.Unlock()
		return nil
	}
	bridge.Protocol.Lock()

	for device := range bridge.Protocol.Devices {
		bridge.Protocol.RemoveDevice(device)
	}
	if !isNil(r.Callbacks) {
		go r.Callbacks.RemoveBridge(bridgeID)
	}
	bridge.Protocol.Unlock()
	delete(r.dc.Bridges, bridgeID)
	r.dc.emitBridgeRemoved(bridgeID)
	r.Protocol.Unlock()
	return nil
}

func (r *RootProto) setLogLevel(c *prop.Change) *dbus.Error {
	loglevel := c.Value.(string)
	level, err := logging.LogLevel(loglevel)
	if err == nil {
		logging.SetLevel(level, r.dc.Log.Module)
		r.log.Info("Log level has been set to ", c.Value.(string))
		return &dbus.ErrMsgInvalidArg
	} else {
		r.log.Error(err)
	}
	return nil
}

func initRootProtocolProp(r *RootProto) map[string]map[string]*prop.Prop {
	return map[string]map[string]*prop.Prop{
		dbusProtocolInterface: {
			propertyLogLevel: {
				Value:    logging.GetLevel(r.log.Module).String(),
				Writable: true,
				Emit:     prop.EmitTrue,
				Callback: r.setLogLevel,
			},
		},
	}
}

func initProtocolProp(p *Protocol) map[string]map[string]*prop.Prop {
	return map[string]map[string]*prop.Prop{
		dbusProtocolInterface: {
			propertyReachabilityState: {
				Value:    p.Reachability,
				Writable: false,
				Emit:     prop.EmitTrue,
				Callback: nil,
			},
		},
	}
}

// SetReachabilityState set the value of the property ReachabilityState
func (p *Protocol) SetReachabilityState(state ReachabilityState) {
	if p.properties == nil {
		return
	}

	oldVariant, err := p.properties.Get(dbusProtocolInterface, propertyReachabilityState)

	if err != nil {
		return
	}

	oldState := oldVariant.Value().(ReachabilityState)
	if oldState == state {
		return
	}

	p.log.Info("propertyReachabilityState of the protocol", p.protocolName, "changed from", oldState, "to", state)
	p.properties.SetMust(dbusProtocolInterface, propertyReachabilityState, state)
}
