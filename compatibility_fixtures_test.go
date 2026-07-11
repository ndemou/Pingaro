package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestHistoryCompatibilityFixturesParse(t *testing.T) {
	tests := []struct {
		path        string
		wantRecord  int
		wantSamples int
		wantAggs    int
	}{
		{path: "internal/history/testdata/pretty-v1.json", wantRecord: 1},
		{path: "internal/history/testdata/line-v1.jsonl", wantRecord: 2},
		{path: "internal/history/testdata/successful-probes-v1.json", wantRecord: 1, wantSamples: 1, wantAggs: 1},
		{path: "internal/history/testdata/lost-probes-v1.json", wantRecord: 1, wantSamples: 1, wantAggs: 1},
		{path: "internal/history/testdata/pingaro-history.json", wantRecord: 1},
	}
	for _, tt := range tests {
		data, err := os.ReadFile(tt.path)
		if err != nil {
			t.Fatalf("ReadFile(%s): %v", tt.path, err)
		}
		records, err := parseHistoryRecords(data)
		if err != nil {
			t.Fatalf("parseHistoryRecords(%s): %v", tt.path, err)
		}
		if len(records) != tt.wantRecord {
			t.Fatalf("%s record count = %d, want %d", tt.path, len(records), tt.wantRecord)
		}
		if tt.wantSamples > 0 && len(records[0].Samples) != tt.wantSamples {
			t.Fatalf("%s sample count = %d, want %d", tt.path, len(records[0].Samples), tt.wantSamples)
		}
		if tt.wantAggs > 0 && len(records[0].Aggregates) != tt.wantAggs {
			t.Fatalf("%s aggregate count = %d, want %d", tt.path, len(records[0].Aggregates), tt.wantAggs)
		}
	}
}

func TestSettingsCompatibilityFixturesNormalize(t *testing.T) {
	tests := []struct {
		path        string
		wantUseType string
		wantTargets string
	}{
		{path: "internal/history/testdata/legacy-settings.json", wantUseType: "Browsing & Email", wantTargets: "1.1.1.1, 8.8.8.8"},
		{path: "internal/history/testdata/old-profile-names-settings.json", wantUseType: "Superhuman Gaming", wantTargets: "gateway"},
		{path: "internal/history/testdata/pingaro.json", wantUseType: "Browsing & Email", wantTargets: "localhost"},
	}
	for _, tt := range tests {
		cfg, ok := readSavedConfig(tt.path)
		if !ok {
			t.Fatalf("readSavedConfig(%s) returned false", tt.path)
		}
		if len(cfg.UseTypes) != 1 || cfg.UseTypes[0] != tt.wantUseType {
			t.Fatalf("%s use types = %v, want [%s]", tt.path, cfg.UseTypes, tt.wantUseType)
		}
		if len(cfg.Groups) != 1 || cfg.Groups[0].Targets != tt.wantTargets {
			t.Fatalf("%s groups = %+v, want one group targeting %q", tt.path, cfg.Groups, tt.wantTargets)
		}
	}
}

func TestLegacyFilenameFixturesExerciseMigrationPaths(t *testing.T) {
	dir := t.TempDir()
	historyPath := filepath.Join(dir, "history.json")
	legacyHistoryPath := filepath.Join(dir, "pingaro-history.json")
	copyFixture(t, "internal/history/testdata/pingaro-history.json", legacyHistoryPath)

	if err := migrateLegacyHistoryFile(historyPath, legacyHistoryPath); err != nil {
		t.Fatalf("migrateLegacyHistoryFile() error = %v", err)
	}
	if _, err := os.Stat(historyPath); err != nil {
		t.Fatalf("history.json was not created: %v", err)
	}
	if _, err := os.Stat(legacyHistoryPath); !os.IsNotExist(err) {
		t.Fatalf("legacy history still exists or stat failed unexpectedly: %v", err)
	}

	settingsPath := filepath.Join(dir, "settings.json")
	legacySettingsPath := filepath.Join(dir, "pingaro.json")
	copyFixture(t, "internal/history/testdata/pingaro.json", legacySettingsPath)
	cfg := loadConfigFromPaths(settingsPath, legacySettingsPath)
	if len(cfg.Groups) != 1 || cfg.Groups[0].Name != "Legacy Settings" {
		t.Fatalf("loaded legacy settings groups = %+v", cfg.Groups)
	}
	if _, err := os.Stat(settingsPath); err != nil {
		t.Fatalf("settings.json was not created: %v", err)
	}
	if _, err := os.Stat(legacySettingsPath); !os.IsNotExist(err) {
		t.Fatalf("legacy settings still exists or stat failed unexpectedly: %v", err)
	}
}

func copyFixture(t *testing.T, src, dst string) {
	t.Helper()
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", src, err)
	}
	if err := os.WriteFile(dst, data, 0644); err != nil {
		t.Fatalf("WriteFile(%s): %v", dst, err)
	}
}
