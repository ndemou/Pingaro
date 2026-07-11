package history

import (
	"time"

	"pingaro/internal/settings"
)

type File struct {
	Version       int             `json:"version"`
	SavedAt       time.Time       `json:"savedAt"`
	Config        settings.Config `json:"config"`
	PeriodSeconds int             `json:"periodSeconds"`
	Samples       []Sample        `json:"samples"`
	Aggregates    []Aggregate     `json:"aggregates"`
}

type Sample struct {
	At          time.Time `json:"at"`
	GroupIndex  int       `json:"groupIndex"`
	GroupName   string    `json:"groupName"`
	RTT         int       `json:"rtt"`
	Lost        bool      `json:"lost"`
	MinRTT      int       `json:"minRtt"`
	MaxRTT      int       `json:"maxRtt"`
	Total       int       `json:"total"`
	LostTotal   int       `json:"lostTotal"`
	LossPercent float64   `json:"lossPercent"`
	P95         float64   `json:"p95"`
	JitterP95   float64   `json:"jitterP95"`
	WindowLoss  float64   `json:"windowLoss"`
}

type Aggregate struct {
	At         time.Time `json:"at"`
	GroupIndex int       `json:"groupIndex"`
	GroupName  string    `json:"groupName"`
	P95        float64   `json:"p95"`
	Loss       float64   `json:"loss"`
	JitterP95  float64   `json:"jitterP95"`
}
