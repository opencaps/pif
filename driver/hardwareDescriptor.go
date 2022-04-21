package driver

// HardwareDescriptor struct for an hardware descriptor from hemis
type HardwareDescriptor struct {
	IsSensor        bool               `json:"sensor"`
	ExtendedType    *string            `json:"extendedType,omitempty"`
	Formula         map[string]Formula `json:"formulas"`
	ValueFirstIndex *int               `json:"valueFirstIndex,omitempty"`
	ValueLastIndex  *int               `json:"valueLastIndex,omitempty"`
	Frequency       *int               `json:"frequency,omitempty"`
	PairingNeeded   bool               `json:"pairingNeeded,omitempty"`

	// For sensor
	RequestFrame *string `json:"requestFrame,omitempty"`

	// For actuator
	AckFrame          *string `json:"ackFrame,omitempty"`
	StateRequestFrame *string `json:"stateRequestFrame,omitempty"`
}

// Formula struct for a formula
type Formula struct {
	Map string   `json:"map"`
	A   *float64 `json:"a,omitempty"`
	B   *float64 `json:"b,omitempty"`
	G   *float64 `json:"g,omitempty"`
}
