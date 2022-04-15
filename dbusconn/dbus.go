package dbusconn

import (
	"os"
	"reflect"

	"github.com/godbus/dbus/v5"
	"github.com/op/go-logging"
)

const (
	driverPath            = "/data/drivers/items/"
	dbusNamePrefix        = "com.ubiant.Protocol."
	dbusPathPrefix        = "/com/ubiant/Devices/"
	dbusProtocolInterface = "com.ubiant.Protocol"
	dbusDeviceInterface   = "com.ubiant.Device"
	dbusItemInterface     = "com.ubiant.Item"
)

// Dbus exported structure
type Dbus struct {
	conn         *dbus.Conn
	RootProtocol RootProto
	Bridges      map[string]*BridgeProto
	ProtocolName string
	Log          *logging.Logger
}

func isNil(i interface{}) bool {
	return i == nil || reflect.ValueOf(i).IsNil()
}

// InitDbus initalizes dbus connection
func (dc *Dbus) InitDbus(protocolName string, cbs interface{}) bool {
	dc.ProtocolName = protocolName
	if dc.Log == nil {
		dc.Log = logging.MustGetLogger("dbus-adapter")
	}
	conn, err := dbus.SystemBus()
	if err != nil {
		dc.Log.Error("Fail to request Dbus systembus", err)
		return false
	}

	dbusName := dbusNamePrefix + dc.ProtocolName
	reply, err := conn.RequestName(dbusName, dbus.NameFlagReplaceExisting|dbus.NameFlagDoNotQueue)
	if err != nil {
		dc.Log.Error("Fail to request Dbus name", err)
		return false
	}

	if reply != dbus.RequestNameReplyPrimaryOwner {
		dc.Log.Warning(os.Stderr, " Dbus name is already taken")
	}

	dc.conn = conn
	dc.Log.Info("Connected on DBus")

	dc.Bridges = map[string]*BridgeProto{}
	return dc.initRootProtocolObject(cbs)
}
