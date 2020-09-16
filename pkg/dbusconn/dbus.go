package dbusconn

import (
	"os"
	"strings"

	"github.com/godbus/dbus"
	"github.com/op/go-logging"
)

const (
	driverPath     = "/data/drivers/items/"
	dbusNamePrefix = "com.ubiant.Radio."
	dbusPathPrefix = "/com/ubiant/Devices/"
	dbusInterface  = "com.ubiant.Devices"
)

// DbusInterface callback called from dbus events
type DbusInterface interface {
	AddDevice(string, string, string, string, map[string]string) bool
}

// Dbus exported structure
type Dbus struct {
	conn      *dbus.Conn
	module    *Module
	Callbacks DbusInterface
	Protocol  string
}

var log = logging.MustGetLogger("dbus-adapter")

// InitDbus dbus initialization
func (dc *Dbus) InitDbus() bool {
	conn, err := dbus.SystemBus()
	if err != nil {
		log.Error("Fail to request Dbus systembus", err)
		return false
	}

	dbusName := dbusNamePrefix + dc.Protocol
	reply, err := conn.RequestName(dbusName, dbus.NameFlagDoNotQueue)
	if err != nil {
		log.Error("Fail to request Dbus name", err)
		return false
	}

	if reply != dbus.RequestNameReplyPrimaryOwner {
		log.Warning(os.Stderr, " Dbus name is already taken")
	}

	dc.conn = conn
	log.Info("Connected on DBus")

	dc.signalListener()

	module, _ := dc.ExportModuleObject(dc.Protocol)
	dc.module = module

	return true
}

func (dc *Dbus) signalListener() {
	sigc := make(chan *dbus.Signal, 1)
	dc.conn.Signal(sigc)

	dc.conn.AddMatchSignal(
		dbus.WithMatchInterface(dbusInterface),
		dbus.WithMatchMember(signalAddDevice))

	go func() {
		for signal := range sigc {
			if !strings.HasSuffix(string(signal.Path), "/"+dc.Protocol) {
				continue
			}

			switch signal.Name {
			case dbusInterface + "." + signalAddDevice:
				dc.handleSignalAddDevice(signal)
			}
		}
	}()
}

// Ready set the Module object parameter "ready" to true
func (dc *Dbus) Ready() {
	if dc.module != nil {
		dc.module.setReady()
	}
}
