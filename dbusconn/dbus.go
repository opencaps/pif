package dbusconn

import (
	"context"
	"encoding/json"
	"os"
	"reflect"
	"time"

	"github.com/godbus/dbus/v5"
	"github.com/op/go-logging"
)

const (
	driverPath               = "/data/drivers/items/"
	dbusNamePrefix           = "com.ubiant.Protocol."
	dbusPathPrefix           = "/com/ubiant/Devices/"
	dbusProtocolInterface    = "com.ubiant.Protocol"
	dbusDeviceInterface      = "com.ubiant.Device"
	dbusItemInterface        = "com.ubiant.Item"
	deviceManagerDestination = "com.ubiant.DeviceManager"
	deviceManagerMethod      = "com.ubiant.DeviceManager.GetStoredDevices"
	deviceManagerPath        = "/com/ubiant/DeviceManager"
	callTimeout              = 12 * time.Second
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

// InitDbus initialization dbus connection
func (dc *Dbus) InitDbus(protocolName string, cbs interface{}) *Protocol {
	dc.ProtocolName = protocolName
	if dc.Log == nil {
		dc.Log = logging.MustGetLogger("dbus-adapter")
	}
	conn, err := dbus.SystemBus()
	if err != nil {
		dc.Log.Error("Fail to request Dbus systembus", err)
		return nil
	}

	dbusName := dbusNamePrefix + dc.ProtocolName
	reply, err := conn.RequestName(dbusName, dbus.NameFlagReplaceExisting|dbus.NameFlagDoNotQueue)
	if err != nil {
		dc.Log.Error("Fail to request Dbus name", err)
		return nil
	}

	if reply != dbus.RequestNameReplyPrimaryOwner {
		dc.Log.Warning(os.Stderr, " Dbus name is already taken")
	}

	dc.conn = conn
	dc.Log.Info("Connected on DBus")

	dc.Bridges = map[string]*BridgeProto{}
	protocol := dc.initRootProtocol(cbs)

	dc.restoreDevices()

	return protocol
}

func (dc *Dbus) restoreDevices() {
	// Get the devices related to this protocol from the DeviceManager
	ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
	defer cancel()

	var ret interface{}
	obj := dc.conn.Object(deviceManagerDestination, deviceManagerPath)
	err := obj.CallWithContext(ctx, deviceManagerMethod, 0, dc.ProtocolName).Store(&ret)
	if err != nil {
		dc.Log.Warning("Unable to get the devices from the DeviceManager: ", err)
		return
	}

	var protocols map[string]interface{}
	json.Unmarshal(ret.([]uint8), &protocols)

	for name, rawProtocol := range protocols {
		var protocol *Protocol
		if name == dc.ProtocolName {
			// This it root protocol
			protocol = dc.RootProtocol.Protocol
		} else {
			// This is bridge protocol
			dc.RootProtocol.AddBridge(name)
			protocol = dc.Bridges[name].Protocol
		}

		devices := rawProtocol.([]map[string]interface{})

		for _, rawDevice := range devices {
			devID := rawDevice["devID"].(string)
			comID := rawDevice["comID"].(string)
			devTypeID := rawDevice["devTypeID"].(string)
			devTypeVersion := rawDevice["typeVersion"].(string)
			devOptions := rawDevice["devOptions"].([]byte)

			protocol.AddDevice(devID, comID, devTypeID, devTypeVersion, devOptions)
			device := protocol.Devices[devID]

			items := rawDevice["items"].([]map[string]interface{})
			for _, rawItem := range items {
				itemID := rawItem["itemID"].(string)
				itemTypeID := rawItem["itemTypeID"].(string)
				itemTypeVersion := rawItem["itemTypeVersion"].(string)
				itemOptions := rawItem["itemOptions"].([]byte)

				device.AddItem(itemID, itemTypeID, itemTypeVersion, itemOptions)
			}
		}

	}
}
