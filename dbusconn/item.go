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
	Device *Device

	ItemID      string
	Mac         string
	TypeID      string
	TypeVersion string
	Options     []byte
	Target      []byte
	Value       []byte

	dc         *Dbus
	properties *prop.Properties
	log        *logging.Logger

	setItemOptionCb interface{ SetItemOptions(*Item) }
	setItemTargetCb interface{ SetItemTarget(*Item, []byte) }
}

func initItem(itemID string, typeID string, typeVersion string, options []byte, d *Device) *Item {
	i := &Item{
		ItemID:      itemID,
		Mac:         d.Address,
		TypeID:      typeID,
		TypeVersion: typeVersion,
		Options:     options,
		log:         d.log,
		Device:      d,
		dc:          d.dc,
	}

	d.Items[itemID] = i

	if i.dc.conn == nil {
		i.dc.Log.Warning("Unable to export dbus object because dbus connection nil")
	}

	path := dbus.ObjectPath(dbusPathPrefix + i.Device.Protocol.protocolName + "/" + i.Device.DevID + "/" + i.ItemID)

	// properties
	propsSpec := map[string]map[string]*prop.Prop{
		dbusItemInterface: {
			propertyOptions: {
				Value:    i.Options,
				Writable: true,
				Emit:     prop.EmitTrue,
				Callback: i.setItemOptions,
			},
			propertyTarget: {
				Value:    i.Target,
				Writable: true,
				Emit:     prop.EmitTrue,
				Callback: i.setItemTarget,
			},
			propertyValue: {
				Value:    i.Value,
				Writable: false,
				Emit:     prop.EmitTrue,
				Callback: nil,
			},
		},
	}
	properties, err := prop.Export(i.dc.conn, path, propsSpec)
	if err == nil {
		i.properties = properties
	} else {
		i.log.Error("Fail to export the properties of the device", i.Device.DevID, i.ItemID, err)
	}

	i.dc.conn.Export(i, path, dbusItemInterface)

	i.SetCallbacks(d.Protocol.cbs)

	if !isNil(d.addItemCB) {
		go d.addItemCB.AddItem(i)
	}

	i.dc.conn.Emit(path, dbusItemInterface+"."+signalItemAdded, []interface{}{i.TypeID, i.TypeVersion, i.Options})

	return i
}

func removeItem(i *Item) {
	d := i.Device
	path := dbus.ObjectPath(dbusPathPrefix + i.Device.Protocol.protocolName + "/" + i.Device.DevID + "/" + i.ItemID)

	if !isNil(i.Device.removeItemCB) {
		go d.removeItemCB.RemoveItem(d.DevID, i.ItemID)
	}
	delete(d.Items, i.ItemID)
	d.dc.conn.Emit(path, dbusItemInterface+"."+signalItemRemoved)
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

// SetOption set the value of the property Option
func (i *Item) SetOption(options []byte) {
	if i.properties == nil {
		return
	}

	oldVariant, err := i.properties.Get(dbusItemInterface, propertyOptions)

	if err != nil {
		return
	}

	oldState := oldVariant.Value().([]byte)
	newState := []byte(options)
	if bytes.Equal(oldState, newState) {
		return
	}

	i.log.Info("propertyOptions of the item", i.ItemID, "changed from", oldState, "to", newState)
	i.properties.SetMust(dbusItemInterface, propertyOptions, newState)
}

// SetValue set the value of the property Value
func (i *Item) SetValue(value []byte) {
	if i.properties == nil {
		return
	}

	oldVariant, err := i.properties.Get(dbusItemInterface, propertyValue)

	if err != nil {
		return
	}

	oldState := oldVariant.Value().([]byte)
	newState := []byte(value)
	if bytes.Equal(oldState, newState) {
		return
	}

	i.log.Info("propertyValue of the item", i.ItemID, "changed from", oldState, "to", newState)
	i.properties.SetMust(dbusItemInterface, propertyValue, newState)
}
