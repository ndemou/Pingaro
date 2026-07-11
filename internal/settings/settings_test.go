package settings

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultGroupsPutGatewayBeforeInternet(t *testing.T) {
	got := DefaultGroups()
	if len(got) != 2 {
		t.Fatalf("len(DefaultGroups) = %d, want 2", len(got))
	}
	if got[0].Name != "Gateway" || got[0].Targets != "gateway" {
		t.Fatalf("group 1 = %+v, want Gateway gateway", got[0])
	}
	if got[1].Name != "Internet" || got[1].Targets != DefaultInternetTargets {
		t.Fatalf("group 2 = %+v, want Internet defaults", got[1])
	}
}

func TestLoadFromPathsMigratesLegacyPingaroJSON(t *testing.T) {
	dir := t.TempDir()
	settingsPath := filepath.Join(dir, "settings.json")
	legacyPath := filepath.Join(dir, "pingaro.json")
	legacy := []byte(`{
  "groups": [{"name": "Gateway", "targets": "gateway"}],
  "pps": 2,
  "aggregationSeconds": 90,
  "useTypes": ["audio calls"]
}`)
	if err := os.WriteFile(legacyPath, legacy, 0644); err != nil {
		t.Fatalf("WriteFile legacy config: %v", err)
	}

	got := LoadFromPaths(settingsPath, legacyPath)
	if got.PPS != 2 || got.AggregationSeconds != 90 {
		t.Fatalf("loaded config timing = pps %d agg %d, want pps 2 agg 90", got.PPS, got.AggregationSeconds)
	}
	if len(got.Groups) != 1 || got.Groups[0].Name != "Gateway" || got.Groups[0].Targets != "gateway" {
		t.Fatalf("loaded groups = %+v, want Gateway gateway", got.Groups)
	}
	if len(got.UseTypes) != 1 || got.UseTypes[0] != "Audio Calls" {
		t.Fatalf("loaded use types = %v, want [Audio Calls]", got.UseTypes)
	}
	if _, err := os.Stat(settingsPath); err != nil {
		t.Fatalf("settings.json was not written: %v", err)
	}
	if _, err := os.Stat(legacyPath); !os.IsNotExist(err) {
		t.Fatalf("legacy pingaro.json still exists or stat failed unexpectedly: %v", err)
	}
}

func TestLoadFromPathsPrefersSettingsJSONOverLegacy(t *testing.T) {
	dir := t.TempDir()
	settingsPath := filepath.Join(dir, "settings.json")
	legacyPath := filepath.Join(dir, "pingaro.json")
	settings := []byte(`{"groups":[{"name":"Settings","targets":"localhost"}],"pps":3,"aggregationSeconds":120,"useTypes":["video calls"]}`)
	legacy := []byte(`{"groups":[{"name":"Legacy","targets":"gateway"}],"pps":1,"aggregationSeconds":60,"useType":"audio calls"}`)
	if err := os.WriteFile(settingsPath, settings, 0644); err != nil {
		t.Fatalf("WriteFile settings config: %v", err)
	}
	if err := os.WriteFile(legacyPath, legacy, 0644); err != nil {
		t.Fatalf("WriteFile legacy config: %v", err)
	}

	got := LoadFromPaths(settingsPath, legacyPath)
	if len(got.Groups) != 1 || got.Groups[0].Name != "Settings" || got.Groups[0].Targets != "localhost" {
		t.Fatalf("loaded groups = %+v, want Settings localhost", got.Groups)
	}
	if _, err := os.Stat(legacyPath); err != nil {
		t.Fatalf("legacy config should be left alone when settings.json exists: %v", err)
	}
}

func TestNormalizeLoadedAllowsFiftyMeasurementsAndOneSecondWindow(t *testing.T) {
	got := NormalizeLoaded(Config{PPS: 50, AggregationSeconds: 1})
	if got.PPS != 50 || got.AggregationSeconds != 1 {
		t.Fatalf("NormalizeLoaded timing = pps %d agg %d, want pps 50 agg 1", got.PPS, got.AggregationSeconds)
	}
}

func TestNormalizeLoadedRequiresTwoSamplesPerAggregation(t *testing.T) {
	got := NormalizeLoaded(Config{PPS: 1, AggregationSeconds: 1})
	if got.PPS != 1 || got.AggregationSeconds != 2 {
		t.Fatalf("NormalizeLoaded timing = pps %d agg %d, want pps 1 agg 2", got.PPS, got.AggregationSeconds)
	}
}

func TestNormalizeLoadedClampsMeasurementsPerSecondToFifty(t *testing.T) {
	got := NormalizeLoaded(Config{PPS: 99, AggregationSeconds: 1})
	if got.PPS != 50 || got.AggregationSeconds != 1 {
		t.Fatalf("NormalizeLoaded timing = pps %d agg %d, want pps 50 agg 1", got.PPS, got.AggregationSeconds)
	}
}

func TestNormalizeGroupsCleansTargetsAndNames(t *testing.T) {
	got := NormalizeGroups([]Group{
		{Name: "", Targets: " localhost; gateway "},
		{Name: "Drop", Targets: " , ; "},
		{Name: "Third", Targets: "1.1.1.1"},
	})
	if len(got) != 2 {
		t.Fatalf("len(NormalizeGroups) = %d, want 2: %+v", len(got), got)
	}
	if got[0].Name != "Group 1" || got[0].Targets != "localhost, gateway" {
		t.Fatalf("group 1 = %+v, want generated name and normalized targets", got[0])
	}
	if got[1].Name != "Third" || got[1].Targets != "1.1.1.1" {
		t.Fatalf("group 2 = %+v, want Third 1.1.1.1", got[1])
	}
}
