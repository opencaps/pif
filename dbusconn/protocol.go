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

// Protocol is a dbus object which represents the states of a protocol
type Protocol struct {
	dc             *Dbus
	Devices        map[string]*Device
	ready          bool
	log            *logging.Logger
	properties     *prop.Properties
	Reachability   ReachabilityState
	protocolName   string
	addDeviceCB    interface{ AddDevice(*Device) }
	removeDeviceCB interface{ RemoveDevice(string) }
	addItemCB      interface{ AddItem(*Item) }
	removeItemCB   interface{ RemoveItem(string, string) }
	cbs            interface{}
	sync.Mutex
}

// RootProtocol is a dbus object which represents the states of the root protocol
type RootProto struct {
	Protocol       *Protocol
	dc             *Dbus
	properties     *prop.Properties
	log            *logging.Logger
	addBridgeCB    interface{ AddBridge(*Protocol) }
	removeBridgeCB interface{ RemoveBridge(string) }
}

// Protocol is a dbus object which represents the states of a bridge protocol
type BridgeProto struct {
	Protocol *Protocol
	dc       *Dbus
}

func (dc *Dbus) initRootProtocolObject(cbs interface{}) bool {
	if dc.conn == nil {
		dc.Log.Warning("Unable to export Protocol dbus object because dbus connection nil")
		return false
	}

	dc.RootProtocol.dc = dc
	dc.RootProtocol.log = dc.Log

	dc.RootProtocol.Protocol = &Protocol{ready: false,
		dc:           dc,
		Devices:      make(map[string]*Device),
		log:          dc.Log,
		protocolName: dc.ProtocolName,
		Reachability: ReachabilityUnknown,
		cbs:          cbs,
	}

	path := dbus.ObjectPath(dbusPathPrefix + dc.ProtocolName)

	// properties
	rootPropsSpec := initRootProtocolProp(&dc.RootProtocol)
	rootProperties, err := prop.Export(dc.conn, path, rootPropsSpec)
	if err == nil {
		dc.RootProtocol.properties = rootProperties
	} else {
		dc.Log.Error("Fail to export the properties of the root protocol", dc.ProtocolName, err)
	}

	exportedMethods := make(map[string]interface{})
	exportedMethods["IsReady"] = dc.RootProtocol.Protocol.IsReady
	exportedMethods["AddBridge"] = dc.RootProtocol.AddBridge
	exportedMethods["RemoveBridge"] = dc.RootProtocol.RemoveBridge
	exportedMethods["AddDevice"] = dc.RootProtocol.Protocol.AddDevice
	exportedMethods["RemoveDevice"] = dc.RootProtocol.Protocol.RemoveDevice

	err = dc.conn.ExportMethodTable(exportedMethods, path, dbusProtocolInterface)
	if err != nil {
		dc.Log.Warning("Fail to export Module dbus object", err)
		return false
	}
	dc.RootProtocol.setRootProtocolCBs()
	dc.RootProtocol.Protocol.setProtocolCBs()
	return true
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
		device.setCallbacks()
		if !isNil(p.addDeviceCB) {
			go p.addDeviceCB.AddDevice(p.Devices[devID])
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
	if !isNil(p.removeDeviceCB) {
		go p.removeDeviceCB.RemoveDevice(devID)
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
		var proto = &Protocol{ready: false,
			dc:           r.dc,
			Devices:      make(map[string]*Device),
			log:          r.log,
			protocolName: protoName,
			Reachability: ReachabilityUnknown,
			cbs:          r.Protocol.cbs,
		}
		path := dbus.ObjectPath(dbusPathPrefix + protoName)
		propsSpec := initProtocolProp(proto)
		properties, err := prop.Export(r.dc.conn, path, propsSpec)
		if err == nil {
			proto.properties = properties
		} else {
			proto.log.Error("Fail to export the properties of the protocol", protoName, err)
			return false, &dbus.Error{Name: "Property export", Body: []interface{}{err}}
		}

		err = r.dc.conn.Export(proto, path, dbusProtocolInterface)
		if err != nil {
			proto.log.Warning("Fail to export Protocol dbus object", err)
			return false, &dbus.Error{Name: "Method export", Body: []interface{}{err}}
		}

		proto.setProtocolCBs()

		var bridge = &BridgeProto{Protocol: proto, dc: r.dc}
		r.dc.Bridges[bridgeID] = bridge
		if !isNil(r.addBridgeCB) {
			go r.addBridgeCB.AddBridge(proto)
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
	if !isNil(r.removeBridgeCB) {
		go r.removeBridgeCB.RemoveBridge(bridgeID)
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
			propertyReachabilityState: {
				Value:    r.Protocol.Reachability,
				Writable: false,
				Emit:     prop.EmitTrue,
				Callback: nil,
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

func (r *RootProto) setRootProtocolCBs() {
	switch cb := r.Protocol.cbs.(type) {
	case interface{ AddBridge(*Protocol) }:
		r.addBridgeCB = cb
	}
	switch cb := r.Protocol.cbs.(type) {
	case interface{ RemoveBridge(string) }:
		r.removeBridgeCB = cb
	}
}

func (p *Protocol) setProtocolCBs() {
	switch cb := p.cbs.(type) {
	case interface{ AddDevice(*Device) }:
		p.addDeviceCB = cb
	}
	switch cb := p.cbs.(type) {
	case interface{ RemoveDevice(string) }:
		p.removeDeviceCB = cb
	}
	switch cb := p.cbs.(type) {
	case interface{ AddItem(*Item) }:
		p.addItemCB = cb
	}
	switch cb := p.cbs.(type) {
	case interface{ RemoveItem(string, string) }:
		p.removeItemCB = cb
	}
}
