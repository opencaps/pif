package driver

import (
	"regexp"
)

const (
	itemsPath = "/data/opencaps/drivers/items/"
)

// DriverItem driver for an item type
type DriverItem struct {
	Type          string
	Read          Translation
	Write         Translation
	Frequency     *int
	IsSensor      bool
	PairingNeeded bool
	HDesc         *HardwareDescriptor
}

var itemPathRegex, _ = regexp.Compile("[^a-zA-Z0-9_]")

func initDriverItem(hd HardwareDescriptor) (*DriverItem, bool) {
	driver := &DriverItem{HDesc: &hd}

	if hd.ExtendedType != nil {
		driver.Type = *hd.ExtendedType
	}

	if hd.IsSensor {
		if hd.RequestFrame != nil {
			driver.Read.Field = *hd.RequestFrame
			driver.Write.Field = *hd.RequestFrame
		}
	} else {
		if hd.AckFrame != nil {
			driver.Read.Field = *hd.AckFrame
		}
		if hd.StateRequestFrame != nil {
			driver.Write.Field = *hd.StateRequestFrame
		} else {
			if hd.AckFrame != nil {
				driver.Write.Field = *hd.AckFrame
			}
		}
	}

	if hd.IsSensor {
		tfStandard, ok := hd.Formula["STANDARD"]
		driver.Read.init(&tfStandard, ok, true)
		driver.Write.init(nil, false, false)
	} else {
		tfStandard, ok := hd.Formula["STANDARD"]
		driver.Write.init(&tfStandard, ok, false)
		tfState, ok := hd.Formula["STATE"]
		driver.Read.init(&tfState, ok, true)
	}

	driver.Frequency = hd.Frequency
	driver.IsSensor = hd.IsSensor
	driver.PairingNeeded = hd.PairingNeeded

	return driver, true
}

func itemPath(id string, version string) string {
	id = itemPathRegex.ReplaceAllString(id, "_")
	version = itemPathRegex.ReplaceAllString(version, "_")

	return itemsPath + id + "-" + version + ".json"
}
