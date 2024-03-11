package dbusconn

import (
	"context"
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"time"

	"github.com/godbus/dbus/v5"
	"github.com/op/go-logging"
)

const (
	dbusNamePrefix             = "io.opencaps.Protocol."
	dbusPathPrefix             = "/io/opencaps/Devices/"
	dbusProtocolInterface      = "io.opencaps.Protocol"
	dbusDeviceInterface        = "io.opencaps.Device"
	dbusItemInterface          = "io.opencaps.Item"
	deviceManagerDestination   = "io.opencaps.DeviceManager"
	deviceManagerDevicesMethod = "io.opencaps.DeviceManager.GetStoredDevices"
	deviceManagerBridgesMethod = "io.opencaps.DeviceManager.GetBridges"
	deviceManagerPath          = "/io/opencaps/DeviceManager"
	callTimeout                = 12 * time.Second
)

// Dbus exported structure
type Dbus struct {
	conn         *dbus.Conn
	RootProtocol RootProto
	Bridges      map[string]*BridgeProto
	ProtocolName string
	Log          *logging.Logger
}

type ProtocolJson struct {
	Protocols map[string][]DeviceJson `json:"Protocols"`
}

type BridgeJson struct {
	Bridges map[string]string `json:"bridges"`
}

type DeviceJson struct {
	DevID          string          `json:"devID"`
	ComID          string          `json:"comID"`
	DevTypeID      string          `json:"devTypeID"`
	DevTypeVersion string          `json:"typeVersion"`
	DevOptions     json.RawMessage `json:"devOptions"`
	Items          []ItemJson      `json:"items"`
}

type ItemJson struct {
	ItemID          string          `json:"itemID"`
	ItemTypeID      string          `json:"itemTypeID"`
	ItemTypeVersion string          `json:"itemTypeVersion"`
	ItemOptions     json.RawMessage `json:"itemOptions"`
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

	dc.restoreBridges()
	dc.restoreDevices()

	return protocol
}

func (dc *Dbus) restoreBridges() {
	// Get the bridges related to this protocol from the DeviceManager
	ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
	defer cancel()

	var ret json.RawMessage
	obj := dc.conn.Object(deviceManagerDestination, deviceManagerPath)
	err := obj.CallWithContext(ctx, deviceManagerBridgesMethod, 0).Store(&ret)
	if err != nil {
		dc.Log.Warning("Unable to get the bridges from the DeviceManager: ", err)
		return
	}
	var bridges BridgeJson
	err = json.Unmarshal(ret, &bridges)
	if err != nil {
		dc.Log.Error("Could not read bridges json from the DeviceManager: ", err)
		return
	}

	for bridgeID, bridgeProtocol := range bridges.Bridges {
		if bridgeProtocol == dc.ProtocolName {
			dc.RootProtocol.AddBridge(bridgeID)
		}
	}
}

func (dc *Dbus) restoreDevices() {
	// Get the devices related to this protocol from the DeviceManager
	ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
	defer cancel()

	var ret json.RawMessage
	obj := dc.conn.Object(deviceManagerDestination, deviceManagerPath)
	err := obj.CallWithContext(ctx, deviceManagerDevicesMethod, 0, dc.ProtocolName).Store(&ret)
	if err != nil {
		dc.Log.Warning("Unable to get the devices from the DeviceManager: ", err)
		return
	}

	var protocols ProtocolJson
	err = json.Unmarshal(ret, &protocols)
	if err != nil {
		dc.Log.Error("Could not read devices json from the DeviceManager: ", err)
		return
	}

	for name, devices := range protocols.Protocols {
		var protocol *Protocol
		if name == dc.ProtocolName {
			// This it root protocol
			protocol = dc.RootProtocol.Protocol
		} else {
			// This is bridge protocol
			bridgeId := strings.ReplaceAll(name, dc.ProtocolName+"_", "")
			dc.RootProtocol.AddBridge(bridgeId)
			protocol = dc.Bridges[bridgeId].Protocol
		}

		for _, dev := range devices {
			protocol.AddDevice(dev.DevID, dev.ComID, dev.DevTypeID, dev.DevTypeVersion, dev.DevOptions)
			device := protocol.Devices[dev.DevID]

			for _, item := range dev.Items {
				device.AddItem(item.ItemID, item.ItemTypeID, item.ItemTypeVersion, item.ItemOptions)
			}
		}
	}
}
