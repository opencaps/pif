package driver

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"sync"

	"github.com/op/go-logging"
)

// DriversManager contains all the driver known by the firmware
type DriversManager struct {
	items map[string]DriverItem
	sync.Mutex
}

var log = logging.MustGetLogger("dbus-adapter")

// InitDriversManager init the the struct
func (dm *DriversManager) InitDriversManager() {
	dm.items = make(map[string]DriverItem)
}

// GetItem to get an item from the struct
func (dm *DriversManager) GetItem(id string, version string) (*DriverItem, bool) {
	dm.Lock()
	name := driverName(id, version)
	driver, driverFound := dm.items[name]
	dm.Unlock()

	return &driver, driverFound
}

// FindItem to get an item
// If the item is not in the struct, the function will try to find it on the disk
func (dm *DriversManager) FindItem(id string, version string) (*DriverItem, bool) {
	driver, driverFound := dm.GetItem(id, version)

	if driverFound {
		return driver, driverFound
	}

	log.Info("Try to find the driver from the disk")

	path := itemPath(id, version)
	jsonFile, err := os.Open(path)
	if err != nil {
		log.Warning("unable to open the item driver from", path)
		return nil, false
	}
	defer jsonFile.Close()

	byteValue, err := ioutil.ReadAll(jsonFile)
	if err != nil {
		log.Warning("unable to read the item driver from", path)
		return nil, false
	}

	driverFound = true

	hd := HardwareDescriptor{}
	err = json.Unmarshal(byteValue, &hd)
	if err != nil {
		log.Warning("Fail to deserialize the hardware descriptor:", id, version, err)
		return nil, false
	}

	driver, ok := initDriverItem(hd)
	if !ok {
		log.Warning("Fail to generate a driver item from the hardware descriptor:", id, version, err)
		return nil, false
	}

	log.Info("Driver from disk:", driver)

	dm.Lock()
	dm.items[driverName(id, version)] = *driver
	dm.Unlock()

	return driver, driverFound
}

func driverName(id string, version string) string {
	return id + version
}
