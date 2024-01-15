package driver

// HardwareDescriptor struct for an hardware descriptor from hemis
type HardwareDescriptor struct {
	IsSensor            bool               `json:"sensor"`
	CommunicationType   *string            `json:"communicationType"`
	Protocol            *string            `json:"protocol"`
	ApplicationProtocol *string            `json:"applicationProtocol"`
	ChannelIndex        *int               `json:"channelIndex,omitempty"`
	EoRorg              *int8              `json:"eoRorg,omitempty"`
	EoFunc              *int8              `json:"eoFunc,omitempty"`
	Type                *int8              `json:"type,omitempty"`
	RemoteLearning      *bool              `json:"remoteLearning,omitempty"`
	UsesTriggers        *bool              `json:"usesTriggers,omitempty"`
	ExtendedType        *string            `json:"extendedType,omitempty"`
	ResetValue          *float64           `json:"resetValue,omitempty"`
	Formula             map[string]Formula `json:"formulas"`
	Frequency           *int               `json:"frequency,omitempty"`
	PairingNeeded       bool               `json:"pairingNeeded,omitempty"`

	// For sensor
	RequestFrame *string `json:"requestFrame,omitempty"`

	// For actuator
	AckFrame          *string `json:"ackFrame,omitempty"`
	StateRequestDelay *int    `json:"stateRequestDelay,omitempty"`
	AutoStateResponse *bool   `json:"autoStateResponse,omitempty"`
	StateRequestFrame *string `json:"stateRequestFrame,omitempty"`
	UrlToPropagate    *string `json:"urlToPropagate,omitempty"`
}

// Formula struct for a formula
type Formula struct {
	FormulaType           *string  `json:"translationType,omitempty"`
	Map                   string   `json:"map"`
	A                     *float64 `json:"a,omitempty"`
	B                     *float64 `json:"b,omitempty"`
	G                     *float64 `json:"g,omitempty"`
	StartWith             *string  `json:"startWith,omitempty"`
	ConstantPart          *string  `json:"constantPart,omitempty"`
	ValueFirstIndex       *int     `json:"valueFirstIndex,omitempty"`
	ValueLastIndex        *int     `json:"valueLastIndex,omitempty"`
	LearnBitIndex         *int     `json:"LRNBIndex,omitempty"`
	LearnBitValue         *int     `json:"learnBitValue,omitempty"`
	DIVFirstIndex         *int     `json:"divFirstIndex,omitempty"`
	DIVLastIndex          *int     `json:"divLastIndex,omitempty"`
	DivMap                *string  `json:"divMap,omitempty"`
	DataTypeIndex         *int     `json:"DTIndex,omitempty"`
	DTIndexLength         *int     `json:"DTIndexLength,omitempty"`
	DataTypeToExtract     *int     `json:"datatypeToExtract,omitempty"`
	ChannelIndex          *int     `json:"channelIndex,omitempty"`
	ChannelIndexLength    *int     `json:"channelIndexLength,omitempty"`
	ChannelIndexToExtract *int     `json:"channelIndexToExtract,omitempty"`
	HasFrameCounter       *bool    `json:"hasFrameCounter,omitempty"`
}
