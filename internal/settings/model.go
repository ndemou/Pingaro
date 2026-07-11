package settings

type Config struct {
	Groups             []Group  `json:"groups"`
	PPS                int      `json:"pps"`
	AggregationSeconds int      `json:"aggregationSeconds"`
	UseType            string   `json:"useType,omitempty"`
	UseTypes           []string `json:"useTypes,omitempty"`
}

type Group struct {
	Name    string `json:"name"`
	Targets string `json:"targets"`
}
