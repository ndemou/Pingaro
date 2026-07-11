package settings

import (
	"fmt"
	"strings"

	"pingaro/internal/profiles"
	"pingaro/internal/targets"
)

const DefaultInternetTargets = "1.1.1.1, 1.1.1.2, 8.8.8.8, 8.8.4.4"

const (
	minPPS                = 1
	maxPPS                = 50
	minAggregationSeconds = 1
	maxAggregationSeconds = 3600
	minSamplesPerWindow   = 2
)

func NormalizeLoaded(cfg Config) Config {
	cfg.PPS = clampInt(cfg.PPS, minPPS, maxPPS)
	cfg.AggregationSeconds = clampInt(cfg.AggregationSeconds, minAggregationSeconds, maxAggregationSeconds)
	if cfg.PPS*cfg.AggregationSeconds < minSamplesPerWindow {
		cfg.AggregationSeconds = clampInt((minSamplesPerWindow+cfg.PPS-1)/cfg.PPS, minAggregationSeconds, maxAggregationSeconds)
	}
	cfg.UseTypes = profiles.NormalizeUseTypes(cfg.UseTypes, cfg.UseType)
	cfg.UseType = ""
	cfg.Groups = NormalizeGroups(cfg.Groups)
	return cfg
}

func Base() Config {
	return Config{PPS: 1, AggregationSeconds: 120, UseTypes: profiles.DefaultUseTypes()}
}

func Default() Config {
	return Config{
		PPS:                1,
		AggregationSeconds: 120,
		UseTypes:           profiles.DefaultUseTypes(),
		Groups:             DefaultGroups(),
	}
}

func DefaultGroups() []Group {
	return []Group{
		{Name: "Gateway", Targets: "gateway"},
		{Name: "Internet", Targets: DefaultInternetTargets},
	}
}

func NormalizeGroups(groups []Group) []Group {
	out := make([]Group, 0, 3)
	for _, g := range groups {
		normalizedTargets := strings.Join(targets.Parse(g.Targets), ", ")
		if normalizedTargets == "" {
			continue
		}
		name := strings.TrimSpace(g.Name)
		if name == "" {
			name = fmt.Sprintf("Group %d", len(out)+1)
		}
		out = append(out, Group{Name: name, Targets: normalizedTargets})
		if len(out) == 3 {
			break
		}
	}
	return out
}

func clampInt(value, minValue, maxValue int) int {
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}
