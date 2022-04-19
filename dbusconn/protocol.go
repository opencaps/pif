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
	cbs            interface{}
	isBridged      bool
	sync.Mutex
}

// RootProtocol is a dbus object which represents the states of the root protocol
type RootProto struct {
	Protocol       *Protocol
	dc             *Dbus
	log            *logging.Logger
	addBridgeCB    interface{ AddBridge(*Protocol) }
	removeBridgeCB interface{ RemoveBridge(string) }
}

// Protocol is a dbus object which represents the states of a bridge protocol
type BridgeProto struct {
	Protocol *Protocol
	dc       *Dbus
}

func (dc *Dbus) initRootProtocol(cbs interface{}) *Protocol {
	if dc.conn == nil {
		dc.Log.Warning("Unable to export Protocol dbus object because dbus connection nil")
		return nil
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
		isBridged:    false,
	}

	if !dc.RootProtocol.Protocol.SetDbusProperties(nil) {
		return nil
	}

	if !dc.RootProtocol.Protocol.SetDbusMethods(nil) {
		return nil
	}

	dc.RootProtocol.SetRootProtocolCBs(cbs)
	dc.RootProtocol.Protocol.SetProtocolCBs(cbs)
	return dc.RootProtocol.Protocol
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

//AddBridge is the dbus method to add a new bridge
func (r *RootProto) AddBridge(bridgeID string) (bool, *dbus.Error) {
	r.log.Info("AddBridge called - bridgeID:", bridgeID)

	protoName := r.dc.ProtocolName + "_" + bridgeID
	r.Protocol.Lock()
	_, alreadyAdded := r.dc.Bridges[bridgeID]
	if !alreadyAdded {
		var p = &Protocol{ready: false,
			dc:           r.dc,
			Devices:      make(map[string]*Device),
			log:          r.log,
			protocolName: protoName,
			Reachability: ReachabilityUnknown,
			cbs:          r.Protocol.cbs,
			isBridged:    true,
		}

		p.SetDbusProperties(nil)
		p.SetDbusMethods(nil)
		p.SetProtocolCBs(p.cbs)

		var bridge = &BridgeProto{Protocol: p, dc: r.dc}
		r.dc.Bridges[bridgeID] = bridge
		if !isNil(r.addBridgeCB) {
			go r.addBridgeCB.AddBridge(p)
		}
		p.EmitDbusSignal(signalBridgeAdded)
	}
	r.Protocol.Unlock()
	return alreadyAdded, nil
}

//AddDevice is the dbus method to add a new device
func (p *Protocol) AddDevice(devID string, comID string, typeID string, typeVersion string, options []byte) (bool, *dbus.Error) {
	p.log.Info("AddDevice called - devID:", devID, "comID:", comID, "typeID:", typeID, "typeVersion:", options, "typeVersion:", options)
	p.Lock()
	_, alreadyAdded := p.Devices[devID]
	if !alreadyAdded {
		initDevice(devID, comID, typeID, typeVersion, options, p)
	}
	p.Unlock()
	return alreadyAdded, nil
}

// IsReady dbus method to know if the protocol is ready or not
func (p *Protocol) IsReady() (bool, *dbus.Error) {
	p.Lock()
	var ready = p.ready
	p.Unlock()
	return ready, nil
}

//RemoveBridge is the dbus method to remove a bridge
func (r *RootProto) RemoveBridge(bridgeID string) *dbus.Error {
	r.log.Info("RemoveBridge called - bridgeID:", bridgeID)
	r.Protocol.Lock()
	bridge, bridgePresent := r.dc.Bridges[bridgeID]

	if !bridgePresent {
		r.Protocol.Unlock()
		return nil
	}

	for device := range bridge.Protocol.Devices {
		bridge.Protocol.RemoveDevice(device)
	}
	bridge.Protocol.Lock()
	if !isNil(r.removeBridgeCB) {
		go r.removeBridgeCB.RemoveBridge(bridgeID)
	}
	bridge.Protocol.Unlock()
	delete(r.dc.Bridges, bridgeID)
	path := dbus.ObjectPath(dbusPathPrefix + bridgeID + "_" + bridgeID)
	r.dc.conn.Emit(path, dbusProtocolInterface+"."+signalBridgeRemoved)
	r.dc.conn.Export(nil, path, dbusProtocolInterface)
	r.Protocol.Unlock()
	return nil
}

//RemoveDevice is the dbus method to remove a device
func (p *Protocol) RemoveDevice(devID string) *dbus.Error {
	p.log.Info("RemoveDevice called - devID:", devID)
	p.Lock()
	d, devicePresent := p.Devices[devID]
	if devicePresent {
		removeDevice(d)
	}
	p.Unlock()
	return nil
}

// EmitDbusSignal emit a dbus signal from protocol object
func (p *Protocol) EmitDbusSignal(sigName string, args ...interface{}) {
	path := dbus.ObjectPath(dbusPathPrefix + p.protocolName)
	p.dc.conn.Emit(path, dbusProtocolInterface+"."+sigName, args...)
}

// Ready set the Protocol object parameter "ready" to true
func (p *Protocol) Ready() {
	if p != nil {
		p.Lock()
		p.ready = true
		p.Unlock()
	}
}

// SetDbusMethods set new dbusMethods for this protocol
func (p *Protocol) SetDbusMethods(externalMethods map[string]interface{}) bool {
	path := dbus.ObjectPath(dbusPathPrefix + p.protocolName)
	exportedMethods := make(map[string]interface{})
	exportedMethods["IsReady"] = p.IsReady
	exportedMethods["AddDevice"] = p.AddDevice
	exportedMethods["RemoveDevice"] = p.RemoveDevice
	if p.isBridged {
		exportedMethods["AddBridge"] = p.dc.RootProtocol.AddBridge
		exportedMethods["RemoveBridge"] = p.dc.RootProtocol.RemoveBridge
	}

	for name, inter := range externalMethods {
		exportedMethods[name] = inter
	}

	err := p.dc.conn.ExportMethodTable(exportedMethods, path, dbusProtocolInterface)
	if err != nil {
		p.dc.Log.Warning("Fail to export protocol dbus object", p.protocolName, err)
		return false
	}
	return true
}

// SetDbusProperties set new DBus properties for this protocol
func (p *Protocol) SetDbusProperties(externalProperties map[string]*prop.Prop) bool {
	path := dbus.ObjectPath(dbusPathPrefix + p.protocolName)
	propsSpec := map[string]map[string]*prop.Prop{
		dbusProtocolInterface: {
			propertyReachabilityState: {
				Value:    p.Reachability,
				Writable: false,
				Emit:     prop.EmitTrue,
				Callback: nil,
			},
		},
	}

	if p.isBridged {
		rootPropsSpec := prop.Prop{
			Value:    logging.GetLevel(p.dc.RootProtocol.log.Module).String(),
			Writable: true,
			Emit:     prop.EmitTrue,
			Callback: p.dc.RootProtocol.setLogLevel,
		}
		propsSpec[dbusProtocolInterface][propertyLogLevel] = &rootPropsSpec
	}

	for pName, pr := range externalProperties {
		propsSpec[dbusProtocolInterface][pName] = pr
	}

	properties, err := prop.Export(p.dc.conn, path, propsSpec)
	if err == nil {
		p.properties = properties
	} else {
		p.log.Error("Fail to export the properties of the protocol", p.protocolName, err)
		return false
	}
	return true
}

// SetCallbacks set new callbacks for this protocol
func (p *Protocol) SetProtocolCBs(cbs interface{}) {
	switch cb := cbs.(type) {
	case interface{ AddDevice(*Device) }:
		p.addDeviceCB = cb
	}
	switch cb := cbs.(type) {
	case interface{ RemoveDevice(string) }:
		p.removeDeviceCB = cb
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

// SetCallbacks set new callbacks for this Root protocol
func (r *RootProto) SetRootProtocolCBs(cbs interface{}) {
	switch cb := cbs.(type) {
	case interface{ AddBridge(*Protocol) }:
		r.addBridgeCB = cb
	}
	switch cb := cbs.(type) {
	case interface{ RemoveBridge(string) }:
		r.removeBridgeCB = cb
	}
}
