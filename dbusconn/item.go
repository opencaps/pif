package dbusconn

import (
	"bytes"

	"github.com/godbus/dbus/v5"
	"github.com/godbus/dbus/v5/prop"
	"github.com/op/go-logging"
)

const (
	signalItemAdded   = "ItemAdded"
	signalItemRemoved = "ItemAdded"
	signalItemChanged = "ItemChanged"

	propertyTarget = "Target"
	propertyValue  = "Value"
)

type internalItemInterface interface {
	setItemOptions(c *prop.Change) *dbus.Error
	setItemTarget(c *prop.Change) *dbus.Error
}

type setItemOptionInterface interface {
	SetItemOptions(c *prop.Change) *dbus.Error
}

type setItemTargetInterface interface {
	SetItemTarget(c *prop.Change) *dbus.Error
}

// Item struct
type Item struct {
	ItemID      string
	Mac         string
	TypeID      string
	TypeVersion string
	Options     map[string]string
	properties  *prop.Properties
	log         *logging.Logger

	internalCB      internalItemInterface
	SetItemOptionCb setItemOptionInterface
	SetItemTargetCb setItemTargetInterface
}

// Payload is a struct of data
type Payload struct {
	Value interface{} `json:"value"`
}

func (i *Item) setItemOptions(c *prop.Change) *dbus.Error {
	if !isNil(i.SetItemOptionCb) {
		return i.SetItemOptionCb.SetItemOptions(c)
	}
	i.log.Warning("No Options")
	return nil
}

func (i *Item) setItemTarget(c *prop.Change) *dbus.Error {
	if !isNil(i.SetItemTargetCb) {
		return i.SetItemTargetCb.SetItemTarget(c)
	}
	i.log.Warning("No Target callback")
	return nil
}

// InitItem to init an Item struct
func InitItem(itemID string, typeID string, typeVersion string, address string, options map[string]string, log *logging.Logger) *Item {
	i := &Item{
		ItemID:      itemID,
		Mac:         address,
		TypeID:      typeID,
		TypeVersion: typeVersion,
		Options:     options,
		log:         log,
	}
	i.internalCB = i
	return i
}

// ExportItemOnDbus export an item on dbus
func (dc *Dbus) ExportItemOnDbus(devID string, item *Item) {
	if dc.conn == nil {
		dc.Log.Warning("Unable to export dbus object because dbus connection nil")
	}

	path := dbus.ObjectPath(dbusPathPrefix + dc.ProtocolName + "/" + devID + "/" + item.ItemID)

	// properties
	propsSpec := initItemProp(item)
	properties, err := prop.Export(dc.conn, path, propsSpec)
	if err == nil {
		item.properties = properties
	} else {
		dc.Log.Error("Fail to export the properties of the device", devID, item.ItemID, err)
	}

	dc.conn.Export(item, path, dbusInterface)
	dc.Log.Debug("Item exported:", path)
}

func initItemProp(item *Item) map[string]map[string]*prop.Prop {
	return map[string]map[string]*prop.Prop{
		dbusInterface: {
			propertyOptions: {
				Value:    item.Options,
				Writable: true,
				Emit:     prop.EmitTrue,
				Callback: item.internalCB.setItemOptions,
			},
			propertyTarget: {
				Value:    []byte{},
				Writable: true,
				Emit:     prop.EmitTrue,
				Callback: item.internalCB.setItemTarget,
			},
			propertyValue: {
				Value:    []byte{},
				Writable: false,
				Emit:     prop.EmitTrue,
				Callback: nil,
			},
		},
	}
}

func (dc *Dbus) emitItemAdded(devID string, itemID string) {
	path := dbus.ObjectPath(dbusPathPrefix + dc.ProtocolName + "/" + devID + "/" + itemID)
	dc.conn.Emit(path, dbusInterface+"."+signalItemAdded)
}

func (dc *Dbus) emitItemRemoved(devID string, itemID string) {
	path := dbus.ObjectPath(dbusPathPrefix + dc.ProtocolName + "/" + devID + "/" + itemID)
	dc.conn.Emit(path, dbusInterface+"."+signalDeviceRemoved)
}

// SetReachabilityState set the value of the property ReachabilityState
func (item *Item) SetValue(value []byte) {
	if item.properties == nil {
		return
	}

	oldVariant, err := item.properties.Get(dbusInterface, propertyValue)

	if err != nil {
		return
	}

	oldState := oldVariant.Value().([]byte)
	newState := []byte(value)
	if bytes.Equal(oldState, newState) {
		return
	}

	item.log.Info("propertyValue of the item", item.ItemID, "changed from", oldState, "to", newState)
	item.properties.SetMust(dbusInterface, propertyValue, newState)
}

// SetOption set the value of the property Option
func (item *Item) SetOption(key string, newValue string) {
	oldVal := "empty"
	if item.properties == nil {
		return
	}

	if val, ok := item.Options[key]; ok {
		if val == newValue {
			return
		}
		oldVal = val
	}

	item.log.Info("Option", key, "of the item", item.ItemID, "changed from", oldVal, "to", newValue)
	item.Options[key] = newValue
	item.properties.SetMust(dbusInterface, propertyOptions, item.Options)
}
