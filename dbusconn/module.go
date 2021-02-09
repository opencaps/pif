package dbusconn

import (
	"sync"

	"github.com/godbus/dbus/v5"
)

const (
	moduleInterface  = "com.ubiant.Radio.Module"
	modulePathPrefix = "/com/ubiant/Radio/"
)

// Module is a dbus object which represents the states of the module
type Module struct {
	ready bool
	sync.Mutex
}

// ExportModuleObject Initializes and exports the Module object on DBus
func (dc *Dbus) ExportModuleObject(protocol string) (*Module, bool) {
	if dc.conn == nil {
		log.Warning("Unable to export Module dbus object because dbus connection nil")
		return nil, false
	}

	var module = &Module{ready: false}

	path := dbus.ObjectPath(modulePathPrefix + protocol)
	err := dc.conn.Export(module, path, moduleInterface)
	if err != nil {
		log.Warning("Fail to export Module dbus object", err)
		return nil, false
	}

	return module, true
}

func (m *Module) setReady() {
	m.Lock()
	m.ready = true
	m.Unlock()
}

// IsReady dbus method to known is the module is ready or not
func (m *Module) IsReady() (bool, *dbus.Error) {
	m.Lock()
	var ready = m.ready
	m.Unlock()

	return ready, nil
}
