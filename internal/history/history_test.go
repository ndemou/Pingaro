package history

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestParseRecordsMixedPrettyAndLineJSON(t *testing.T) {
	data := []byte(`{
  "version": 1,
  "savedAt": "2026-06-24T10:10:00+03:00",
  "config": {
    "groups": [
      {"name": "Internet", "targets": "1.1.1.1"}
    ],
    "pps": 1,
    "aggregationSeconds": 120,
    "useType": "email & browsing"
  },
  "periodSeconds": 120,
  "samples": [],
  "aggregates": []
}
{"version":1,"savedAt":"2026-06-24T10:20:00+03:00","config":{"groups":[{"name":"Internet","targets":"1.1.1.1"}],"pps":1,"aggregationSeconds":120,"useType":"email & browsing"},"periodSeconds":120,"samples":[],"aggregates":[]}
`)

	records, err := ParseRecords(data)
	if err != nil {
		t.Fatalf("ParseRecords() error = %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("len(records) = %d, want 2", len(records))
	}
}

func TestMigrateLegacyFileRenamesDefaultHistory(t *testing.T) {
	dir := t.TempDir()
	historyPath := filepath.Join(dir, "history.json")
	legacyPath := filepath.Join(dir, "pingaro-history.json")
	legacy := []byte(`{"version":1,"savedAt":"2026-06-24T10:20:00+03:00","periodSeconds":120,"samples":[],"aggregates":[]}` + "\n")
	if err := os.WriteFile(legacyPath, legacy, 0644); err != nil {
		t.Fatalf("WriteFile legacy history: %v", err)
	}

	if err := MigrateLegacyFile(historyPath, legacyPath); err != nil {
		t.Fatalf("MigrateLegacyFile() error = %v", err)
	}
	got, err := os.ReadFile(historyPath)
	if err != nil {
		t.Fatalf("history.json was not written: %v", err)
	}
	if string(got) != string(legacy) {
		t.Fatalf("history.json content = %q, want %q", got, legacy)
	}
	if _, err := os.Stat(legacyPath); !os.IsNotExist(err) {
		t.Fatalf("legacy pingaro-history.json still exists or stat failed unexpectedly: %v", err)
	}
}

func TestMigrateLegacyFileLeavesLegacyWhenHistoryExists(t *testing.T) {
	dir := t.TempDir()
	historyPath := filepath.Join(dir, "history.json")
	legacyPath := filepath.Join(dir, "pingaro-history.json")
	if err := os.WriteFile(historyPath, []byte("new\n"), 0644); err != nil {
		t.Fatalf("WriteFile history: %v", err)
	}
	if err := os.WriteFile(legacyPath, []byte("legacy\n"), 0644); err != nil {
		t.Fatalf("WriteFile legacy history: %v", err)
	}

	if err := MigrateLegacyFile(historyPath, legacyPath); err != nil {
		t.Fatalf("MigrateLegacyFile() error = %v", err)
	}
	if _, err := os.Stat(legacyPath); err != nil {
		t.Fatalf("legacy history should be left alone when history.json exists: %v", err)
	}
	got, err := os.ReadFile(historyPath)
	if err != nil {
		t.Fatalf("ReadFile history: %v", err)
	}
	if string(got) != "new\n" {
		t.Fatalf("history.json content = %q, want new", got)
	}
}

func TestCreateAutosaveFileUsesTimestampName(t *testing.T) {
	dir := t.TempDir()
	startedAt := time.Date(2026, time.July, 9, 8, 52, 31, 900*int(time.Millisecond), time.Local)

	path, err := CreateAutosaveFile(dir, startedAt, 1234)
	if err != nil {
		t.Fatalf("CreateAutosaveFile() error = %v", err)
	}
	if got, want := filepath.Base(path), "history-2026-07-09_08.52.31.json"; got != want {
		t.Fatalf("autosave filename = %q, want %q", got, want)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("autosave file was not created: %v", err)
	}
}

func TestCreateAutosaveFileAddsPidAndSuffixWhenTimestampExists(t *testing.T) {
	dir := t.TempDir()
	startedAt := time.Date(2026, time.July, 9, 8, 52, 31, 0, time.Local)
	base := filepath.Join(dir, "history-2026-07-09_08.52.31.json")
	if err := os.WriteFile(base, []byte("{}\n"), 0644); err != nil {
		t.Fatalf("WriteFile base autosave: %v", err)
	}

	path, err := CreateAutosaveFile(dir, startedAt, 1234)
	if err != nil {
		t.Fatalf("CreateAutosaveFile() pid path error = %v", err)
	}
	if got, want := filepath.Base(path), "history-2026-07-09_08.52.31-pid1234.json"; got != want {
		t.Fatalf("autosave filename = %q, want %q", got, want)
	}

	path, err = CreateAutosaveFile(dir, startedAt, 1234)
	if err != nil {
		t.Fatalf("CreateAutosaveFile() suffix path error = %v", err)
	}
	if got, want := filepath.Base(path), "history-2026-07-09_08.52.31-pid1234-2.json"; got != want {
		t.Fatalf("autosave filename = %q, want %q", got, want)
	}
}
