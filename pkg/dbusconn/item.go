package dbusconn

import (
	"encoding/json"

	"github.com/godbus/dbus"
)

const (
	signalItemChanged = "ItemChanged"
)

// ItemInterface callback called from item events
type ItemInterface interface {
	SetItem(*Item, []byte) bool
	SetOptionsItem(*Item) bool
}

// Item struct
type Item struct {
	ItemID      string
	Mac         string
	TypeID      string
	TypeVersion string
	Options     map[string]string
	callbacks   ItemInterface
}

// Payload is a struct of data
type Payload struct {
	Value interface{} `json:"value"`
}

// InitItem to init an Item struct
func InitItem(itemID string, typeID string, typeVersion string, address string, options map[string]string, callbacks ItemInterface) *Item {
	return &Item{
		ItemID:      itemID,
		Mac:         address,
		TypeID:      typeID,
		TypeVersion: typeVersion,
		Options:     options,
		callbacks:   callbacks,
	}
}

// ExportItemOnDbus export an item on dbus
func (dc *Dbus) ExportItemOnDbus(devID string, item *Item) {
	if dc.conn == nil {
		log.Warning("Unable to export dbus object because dbus connection nil")
	}

	path := dbus.ObjectPath(dbusPathPrefix + dc.Protocol + "/" + devID + "/" + item.ItemID)
	dc.conn.Export(item, path, dbusInterface)
	log.Info("Item exported:", path)
}

// SetItem called when a new order come from Hemis
func (item *Item) SetItem(order []byte) (bool, *dbus.Error) {
	return item.callbacks.SetItem(item, order), nil
}

// SetOptions called when a new options come from Hemis
func (item *Item) SetOptions(options map[string]string) (bool, *dbus.Error) {
	item.Options = options
	return item.callbacks.SetOptionsItem(item), nil
}

// EmitItemChanged to call when the value of an item has changed
func (dc *Dbus) EmitItemChanged(devID string, itemID string, data *Payload) {
	json, err := json.Marshal(data)
	if err != nil {
		log.Warning("ItemChanged fail to create json", data, err)
		return
	}
	bytes := []byte(string(json))

	path := dbus.ObjectPath(dbusPathPrefix + dc.Protocol)
	dc.conn.Emit(path, dbusInterface+"."+signalItemChanged, devID, itemID, bytes)
}
