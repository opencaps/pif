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

type setItemOptionInterface interface {
	SetItemOptions(*Item) *dbus.Error
}

type setItemTargetInterface interface {
	SetItemTarget(*Item, []byte) *dbus.Error
}

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

	SetItemOptionCb setItemOptionInterface
	SetItemTargetCb setItemTargetInterface
}

func (i *Item) setItemOptions(c *prop.Change) *dbus.Error {
	if !isNil(i.SetItemOptionCb) {
		go i.SetItemOptionCb.SetItemOptions(i)
	} else {
		i.log.Warning("No Options")
	}
	return nil
}

func (i *Item) setItemTarget(c *prop.Change) *dbus.Error {
	if !isNil(i.SetItemTargetCb) {
		go i.SetItemTargetCb.SetItemTarget(i, c.Value.([]byte))
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

func (dc *Dbus) exportItemOnDbus(devID string, item *Item) {
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

func (dc *Dbus) emitItemAdded(devID string, item *Item) {
	args := make([]interface{}, 3)
	args[0] = item.TypeID
	args[1] = item.TypeVersion
	args[2] = item.Options
	path := dbus.ObjectPath(dbusPathPrefix + dc.ProtocolName + "/" + devID + "/" + item.ItemID)
	dc.conn.Emit(path, dbusItemInterface+"."+signalItemAdded, args...)
}

func (dc *Dbus) emitItemRemoved(devID string, itemID string) {
	path := dbus.ObjectPath(dbusPathPrefix + dc.ProtocolName + "/" + devID + "/" + itemID)
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
