package dto

// TrafficPoint encodes an epoch-millisecond timestamp and a nullable rate.
// A nil rate is an explicit collection gap, not an observed zero.
type TrafficPoint [2]any
type CircuitTrafficSeries struct {
	Side         string         `json:"side"`
	Direction    string         `json:"direction"`
	HostID       string         `json:"hostId"`
	ItemID       string         `json:"itemId"`
	Name         string         `json:"name"`
	HostName     string         `json:"hostName"`
	Interface    string         `json:"interface"`
	State        string         `json:"state"`
	Mode         string         `json:"mode"`
	CurrentBps   *float64       `json:"currentBps"`
	PeakBps      *float64       `json:"peakBps"`
	LastSampleAt *int64         `json:"lastSampleAt"`
	Partial      bool           `json:"partial"`
	Points       []TrafficPoint `json:"points"`
}
type CircuitTraffic struct {
	Configured bool                   `json:"configured"`
	Period     string                 `json:"period"`
	TimeFrom   int64                  `json:"timeFrom"`
	TimeTill   int64                  `json:"timeTill"`
	Series     []CircuitTrafficSeries `json:"series"`
}
