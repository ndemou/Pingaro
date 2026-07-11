package profiles

import (
	"math"
	"strings"
)

type Severity uint8

const (
	SeverityGood Severity = iota
	SeverityMinor
	SeverityNoticeable
	SeveritySerious
)

type Profile struct {
	Name            string
	RTT             [3]float64
	Loss            [3]float64
	Jitter          [3]float64
	DefaultSelected bool
	ShowsJitter     bool
}

var allProfiles = []Profile{
	{Name: "Browsing & Email", RTT: [3]float64{150, 300, 600}, Loss: [3]float64{0.5, 2, 5}, Jitter: [3]float64{600, 600, 600}, DefaultSelected: true},
	{Name: "Remote Desktop", RTT: [3]float64{100, 150, 220}, Loss: [3]float64{1, 2, 3}, Jitter: [3]float64{15, 30, 50}},
	{Name: "Audio Calls", RTT: [3]float64{100, 150, 250}, Loss: [3]float64{1, 2, 3}, Jitter: [3]float64{20, 30, 50}, DefaultSelected: true, ShowsJitter: true},
	{Name: "Video Calls", RTT: [3]float64{100, 150, 250}, Loss: [3]float64{1, 2, 3}, Jitter: [3]float64{20, 30, 50}, DefaultSelected: true, ShowsJitter: true},
	{Name: "Online Gaming", RTT: [3]float64{50, 80, 120}, Loss: [3]float64{0.5, 1, 2}, Jitter: [3]float64{10, 20, 30}, DefaultSelected: true},
	{Name: "Superhuman Gaming", RTT: [3]float64{20, 35, 60}, Loss: [3]float64{0.1, 0.5, 1}, Jitter: [3]float64{5, 10, 20}},
}

var aliases = map[string]string{
	"email & browsing":   "Browsing & Email",
	"low latency gaming": "Superhuman Gaming",
}

func All() []Profile {
	return append([]Profile(nil), allProfiles...)
}

func Names() []string {
	names := make([]string, 0, len(allProfiles))
	for _, profile := range allProfiles {
		names = append(names, profile.Name)
	}
	return names
}

func DefaultUseTypes() []string {
	names := make([]string, 0, len(allProfiles))
	for _, profile := range allProfiles {
		if profile.DefaultSelected {
			names = append(names, profile.Name)
		}
	}
	return names
}

func NormalizeUseType(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if name, ok := aliases[value]; ok {
		return name
	}
	for _, profile := range allProfiles {
		if value == strings.ToLower(profile.Name) {
			return profile.Name
		}
	}
	return ""
}

func NormalizeUseTypes(values []string, legacyValue string) []string {
	seen := map[string]bool{}
	var normalized []string
	for _, value := range values {
		name := NormalizeUseType(value)
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		normalized = append(normalized, name)
	}
	if len(normalized) == 0 && strings.TrimSpace(legacyValue) != "" {
		if name := NormalizeUseType(legacyValue); name != "" {
			normalized = append(normalized, name)
		}
	}
	if len(normalized) == 0 {
		return DefaultUseTypes()
	}
	return normalized
}

func ForName(name string) (Profile, bool) {
	name = NormalizeUseType(name)
	for _, profile := range allProfiles {
		if profile.Name == name {
			return profile, true
		}
	}
	return Profile{}, false
}

func ForUses(names []string) Profile {
	names = NormalizeUseTypes(names, "")
	first, _ := ForName(names[0])
	combined := first
	combined.Name = strings.Join(names, ", ")
	for _, name := range names[1:] {
		profile, ok := ForName(name)
		if !ok {
			continue
		}
		for i := 0; i < 3; i++ {
			combined.RTT[i] = math.Min(combined.RTT[i], profile.RTT[i])
			combined.Loss[i] = math.Min(combined.Loss[i], profile.Loss[i])
			combined.Jitter[i] = math.Min(combined.Jitter[i], profile.Jitter[i])
		}
		combined.ShowsJitter = combined.ShowsJitter || profile.ShowsJitter
	}
	return combined
}

func UsesShowJitter(names []string) bool {
	for _, name := range NormalizeUseTypes(names, "") {
		if profile, ok := ForName(name); ok && profile.ShowsJitter {
			return true
		}
	}
	return false
}

func ThresholdSeverity(value float64, thresholds [3]float64) Severity {
	if value >= thresholds[2] {
		return SeveritySerious
	}
	if value >= thresholds[1] {
		return SeverityNoticeable
	}
	if value >= thresholds[0] {
		return SeverityMinor
	}
	return SeverityGood
}
