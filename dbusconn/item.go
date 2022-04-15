package dbusconn

import (
	"bytes"

	"github.com/godbus/dbus/v5"
	"github.com/godbus/dbus/v5/prop"
	"github.com/op/go-logging"
)

const (
	signalItemAdded   = "ItemAdded"
	signalItemRemoved = "ItemRemoved"

	propertyTarget = "Target"
	propertyValue  = "Value"
)

// Item object structure
type Item struct {
	ItemID      string
	Mac         string
	TypeID      string
	TypeVersion string
	Options     []byte
	Target      []byte
	Value       []byte
	properties  *prop.Properties
	log         *logging.Logger
	Device      *Device

	setItemOptionCb interface{ SetItemOptions(*Item) }
	setItemTargetCb interface{ SetItemTarget(*Item, []byte) }
}

func (i *Item) setItemOptions(c *prop.Change) *dbus.Error {
	if !isNil(i.setItemOptionCb) {
		go i.setItemOptionCb.SetItemOptions(i)
	} else {
		i.log.Warning("No Options")
	}
	return nil
}

func (i *Item) setItemTarget(c *prop.Change) *dbus.Error {
	if !isNil(i.setItemTargetCb) {
		go i.setItemTargetCb.SetItemTarget(i, c.Value.([]byte))
	} else {
		i.log.Warning("No Target callback")
	}
	return nil
}

func initItem(itemID string, typeID string, typeVersion string, options []byte, dev *Device) *Item {
	return &Item{
		ItemID:      itemID,
		Mac:         dev.Address,
		TypeID:      typeID,
		TypeVersion: typeVersion,
		Options:     options,
		log:         dev.log,
		Device:      dev,
	}
}

func (dc *Dbus) exportItemOnDbus(item *Item) {
	if dc.conn == nil {
		dc.Log.Warning("Unable to export dbus object because dbus connection nil")
	}

	path := dbus.ObjectPath(dbusPathPrefix + item.Device.Protocol.protocolName + "/" + item.Device.DevID + "/" + item.ItemID)

	// properties
	propsSpec := initItemProp(item)
	properties, err := prop.Export(dc.conn, path, propsSpec)
	if err == nil {
		item.properties = properties
	} else {
		dc.Log.Error("Fail to export the properties of the device", item.Device.DevID, item.ItemID, err)
	}

	dc.conn.Export(item, path, dbusItemInterface)
	dc.Log.Debug("Item exported:", path)
}

func initItemProp(item *Item) map[string]map[string]*prop.Prop {
	return map[string]map[string]*prop.Prop{
		dbusItemInterface: {
			propertyOptions: {
				Value:    item.Options,
				Writable: true,
				Emit:     prop.EmitTrue,
				Callback: item.setItemOptions,
			},
			propertyTarget: {
				Value:    item.Target,
				Writable: true,
				Emit:     prop.EmitTrue,
				Callback: item.setItemTarget,
			},
			propertyValue: {
				Value:    item.Value,
				Writable: false,
				Emit:     prop.EmitTrue,
				Callback: nil,
			},
		},
	}
}

func (dc *Dbus) emitItemAdded(item *Item) {
	args := make([]interface{}, 3)
	args[0] = item.TypeID
	args[1] = item.TypeVersion
	args[2] = item.Options
	path := dbus.ObjectPath(dbusPathPrefix + item.Device.Protocol.protocolName + "/" + item.Device.DevID + "/" + item.ItemID)
	dc.conn.Emit(path, dbusItemInterface+"."+signalItemAdded, args...)
}

func (dc *Dbus) emitItemRemoved(item *Item) {
	path := dbus.ObjectPath(dbusPathPrefix + item.Device.Protocol.protocolName + "/" + item.Device.DevID + "/" + item.ItemID)
	dc.conn.Emit(path, dbusItemInterface+"."+signalItemRemoved)
}

// SetValue set the value of the property Value
func (item *Item) SetValue(value []byte) {
	if item.properties == nil {
		return
	}

	oldVariant, err := item.properties.Get(dbusItemInterface, propertyValue)

	if err != nil {
		return
	}

	oldState := oldVariant.Value().([]byte)
	newState := []byte(value)
	if bytes.Equal(oldState, newState) {
		return
	}

	item.log.Info("propertyValue of the item", item.ItemID, "changed from", oldState, "to", newState)
	item.properties.SetMust(dbusItemInterface, propertyValue, newState)
}

// SetOption set the value of the property Option
func (item *Item) SetOption(options []byte) {
	if item.properties == nil {
		return
	}

	oldVariant, err := item.properties.Get(dbusItemInterface, propertyOptions)

	if err != nil {
		return
	}

	oldState := oldVariant.Value().([]byte)
	newState := []byte(options)
	if bytes.Equal(oldState, newState) {
		return
	}

	item.log.Info("propertyOptions of the item", item.ItemID, "changed from", oldState, "to", newState)
	item.properties.SetMust(dbusItemInterface, propertyOptions, newState)
}

// SetCallbacks set new callbacks for this item
func (i *Item) SetCallbacks(cbs interface{}) {
	switch cb := cbs.(type) {
	case interface{ SetItemOptions(*Item) }:
		i.setItemOptionCb = cb
	}
	switch cb := cbs.(type) {
	case interface{ SetItemTarget(*Item, []byte) }:
		i.setItemTargetCb = cb
	}
}

// SetDbusMethods set new dbusMethods for this Item
func (i *Item) SetDbusMethods(externalMethods map[string]interface{}) bool {
	path := dbus.ObjectPath(dbusPathPrefix + i.Device.Protocol.protocolName + "/" + i.Device.DevID + "/" + i.ItemID)
	err := i.Device.Protocol.dc.conn.ExportMethodTable(externalMethods, path, dbusItemInterface)
	if err != nil {
		i.log.Warning("Fail to export item dbus object", i.ItemID, err)
		return false
	}
	return true
}
