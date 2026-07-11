package main

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lxn/walk"
	"github.com/lxn/win"
)

func TestParseHistoryRecordsMixedPrettyAndLineJSON(t *testing.T) {
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

	records, err := parseHistoryRecords(data)
	if err != nil {
		t.Fatalf("parseHistoryRecords() error = %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("len(records) = %d, want 2", len(records))
	}
}

func TestWindowIconResourceExists(t *testing.T) {
	icon, err := walk.NewIconFromResourceId(appIconResourceID)
	if err != nil {
		t.Fatalf("NewIconFromResourceId(%d) error = %v", appIconResourceID, err)
	}
	icon.Dispose()
}

func TestFormatBuildInfoFormatsRFC3339BuildTimeAsLocal(t *testing.T) {
	got := formatBuildInfo("v1.2.3", "2026-07-09T10:38:56+03:00", time.Time{})
	want := "v1.2.3 - built 2026-07-09 10:38:56"
	if got != want {
		t.Fatalf("formatBuildInfo() = %q, want %q", got, want)
	}
}

func TestFormatBuildInfoFallsBackToExecutableTime(t *testing.T) {
	fallback := time.Date(2026, time.July, 9, 10, 38, 56, 0, time.Local)
	got := formatBuildInfo("", "", fallback)
	want := "dev - built 2026-07-09 10:38:56"
	if got != want {
		t.Fatalf("formatBuildInfo() = %q, want %q", got, want)
	}
}

func TestDefaultGroupColors(t *testing.T) {
	got := defaultGroupColors()
	want := []walk.Color{
		walk.RGB(0, 154, 222),
		walk.RGB(175, 89, 186),
		walk.RGB(255, 31, 91),
	}
	if len(got) != len(want) {
		t.Fatalf("len(defaultGroupColors) = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("group color %d = %#x, want %#x", i+1, uint32(got[i]), uint32(want[i]))
		}
	}
}

func TestHResultErrorDoesNotExposeWalkStack(t *testing.T) {
	got := hresultError("IFileDialog.Show", win.HRESULT(-2147023673)).Error()
	if !strings.Contains(got, "HRESULT 0x800704C7") {
		t.Fatalf("hresultError() = %q, want HRESULT", got)
	}
	if strings.Contains(got, "Stack:") {
		t.Fatalf("hresultError() exposed stack: %q", got)
	}
}

func TestIsCancelledHResult(t *testing.T) {
	if !isCancelledHResult(win.HRESULT(-2147023673)) {
		t.Fatalf("expected cancel HRESULT to be recognized")
	}
	if isCancelledHResult(win.S_OK) {
		t.Fatalf("S_OK must not be treated as cancellation")
	}
}

func TestMeasurementStatusTextUsesSamples(t *testing.T) {
	got := measurementStatusText(12, 5*time.Second)
	want := "12 samples, 5 secs"
	if got != want {
		t.Fatalf("measurementStatusText() = %q, want %q", got, want)
	}
}

func TestGroupInputLabelsIncludeGroupNumber(t *testing.T) {
	if got := groupNameLabel(0); got != "Group 1 name" {
		t.Fatalf("groupNameLabel(0) = %q", got)
	}
	if got := groupTargetsLabel(2); got != "Target(s)" {
		t.Fatalf("groupTargetsLabel(2) = %q", got)
	}
}

func TestEscapeMnemonicPreservesVisibleAmpersand(t *testing.T) {
	if got := escapeMnemonic("Browsing & Email"); got != "Browsing && Email" {
		t.Fatalf("escapeMnemonic() = %q", got)
	}
}

func TestSamplesPerAggregationRequiresAtLeastTwoSamples(t *testing.T) {
	if got := samplesPerAggregation(1, 1); got != 1 {
		t.Fatalf("samplesPerAggregation(1, 1) = %d, want 1", got)
	}
	if got := aggregationSecondsForSamples(1, minSamplesPerAggregation); got != 2 {
		t.Fatalf("aggregationSecondsForSamples(1, 2) = %d, want 2", got)
	}
	if got := samplesPerAggregation(2, 1); got != 2 {
		t.Fatalf("samplesPerAggregation(2, 1) = %d, want 2", got)
	}
}

func TestRenderThrottleDelayCapsRenderingAtTenPerSecond(t *testing.T) {
	now := time.Date(2026, time.July, 11, 12, 0, 0, 0, time.Local)
	if got := renderThrottleDelay(time.Time{}, now); got != 0 {
		t.Fatalf("initial render delay = %v, want 0", got)
	}
	if got := renderThrottleDelay(now, now.Add(50*time.Millisecond)); got != 50*time.Millisecond {
		t.Fatalf("mid-window render delay = %v, want 50ms", got)
	}
	if got := renderThrottleDelay(now, now.Add(100*time.Millisecond)); got != 0 {
		t.Fatalf("boundary render delay = %v, want 0", got)
	}
}

func TestChartLineColorAllowsBlack(t *testing.T) {
	fallback := walk.RGB(40, 150, 135)
	if got := chartLineColor(map[int]walk.Color{0: walk.RGB(0, 0, 0)}, 0, fallback); got != walk.RGB(0, 0, 0) {
		t.Fatalf("chartLineColor() = %#x, want black", uint32(got))
	}
	if got := chartLineColor(map[int]walk.Color{}, 0, fallback); got != fallback {
		t.Fatalf("missing chartLineColor() = %#x, want fallback %#x", uint32(got), uint32(fallback))
	}
}

func TestSeverityColors(t *testing.T) {
	tests := []struct {
		severity int
		want     walk.Color
	}{
		{severity: 1, want: walk.RGB(193, 193, 200)},
		{severity: 2, want: walk.RGB(252, 234, 144)},
		{severity: 3, want: walk.RGB(248, 174, 175)},
	}
	for _, tt := range tests {
		if got := severityColor(tt.severity); got != tt.want {
			t.Fatalf("severityColor(%d) = %#x, want %#x", tt.severity, uint32(got), uint32(tt.want))
		}
		if got := summaryBackgroundColor(tt.severity); got != tt.want {
			t.Fatalf("summaryBackgroundColor(%d) = %#x, want %#x", tt.severity, uint32(got), uint32(tt.want))
		}
	}
}

func TestDefaultGroupsPutGatewayBeforeInternet(t *testing.T) {
	got := defaultGroups()
	if len(got) != 2 {
		t.Fatalf("len(defaultGroups) = %d, want 2", len(got))
	}
	if got[0].Name != "Gateway" || got[0].Targets != "gateway" {
		t.Fatalf("group 1 = %+v, want Gateway gateway", got[0])
	}
	if got[1].Name != "Internet" || got[1].Targets != defaultInternetTargets {
		t.Fatalf("group 2 = %+v, want Internet defaults", got[1])
	}
}

func TestLoadConfigMigratesLegacyPingaroJSON(t *testing.T) {
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

	got := loadConfigFromPaths(settingsPath, legacyPath)
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

func TestLoadConfigPrefersSettingsJSONOverLegacy(t *testing.T) {
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

	got := loadConfigFromPaths(settingsPath, legacyPath)
	if len(got.Groups) != 1 || got.Groups[0].Name != "Settings" || got.Groups[0].Targets != "localhost" {
		t.Fatalf("loaded groups = %+v, want Settings localhost", got.Groups)
	}
	if _, err := os.Stat(legacyPath); err != nil {
		t.Fatalf("legacy config should be left alone when settings.json exists: %v", err)
	}
}

func TestMigrateLegacyHistoryFileRenamesDefaultHistory(t *testing.T) {
	dir := t.TempDir()
	historyPath := filepath.Join(dir, "history.json")
	legacyPath := filepath.Join(dir, "pingaro-history.json")
	legacy := []byte(`{"version":1,"savedAt":"2026-06-24T10:20:00+03:00","periodSeconds":120,"samples":[],"aggregates":[]}` + "\n")
	if err := os.WriteFile(legacyPath, legacy, 0644); err != nil {
		t.Fatalf("WriteFile legacy history: %v", err)
	}

	if err := migrateLegacyHistoryFile(historyPath, legacyPath); err != nil {
		t.Fatalf("migrateLegacyHistoryFile() error = %v", err)
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

func TestMigrateLegacyHistoryFileLeavesLegacyWhenHistoryExists(t *testing.T) {
	dir := t.TempDir()
	historyPath := filepath.Join(dir, "history.json")
	legacyPath := filepath.Join(dir, "pingaro-history.json")
	if err := os.WriteFile(historyPath, []byte("new\n"), 0644); err != nil {
		t.Fatalf("WriteFile history: %v", err)
	}
	if err := os.WriteFile(legacyPath, []byte("legacy\n"), 0644); err != nil {
		t.Fatalf("WriteFile legacy history: %v", err)
	}

	if err := migrateLegacyHistoryFile(historyPath, legacyPath); err != nil {
		t.Fatalf("migrateLegacyHistoryFile() error = %v", err)
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

func TestCreateAutosaveHistoryFileUsesTimestampName(t *testing.T) {
	dir := t.TempDir()
	startedAt := time.Date(2026, time.July, 9, 8, 52, 31, 900*int(time.Millisecond), time.Local)

	path, err := createAutosaveHistoryFile(dir, startedAt, 1234)
	if err != nil {
		t.Fatalf("createAutosaveHistoryFile() error = %v", err)
	}
	if got, want := filepath.Base(path), "history-2026-07-09_08.52.31.json"; got != want {
		t.Fatalf("autosave filename = %q, want %q", got, want)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("autosave file was not created: %v", err)
	}
}

func TestCreateAutosaveHistoryFileAddsPidAndSuffixWhenTimestampExists(t *testing.T) {
	dir := t.TempDir()
	startedAt := time.Date(2026, time.July, 9, 8, 52, 31, 0, time.Local)
	base := filepath.Join(dir, "history-2026-07-09_08.52.31.json")
	if err := os.WriteFile(base, []byte("{}\n"), 0644); err != nil {
		t.Fatalf("WriteFile base autosave: %v", err)
	}

	path, err := createAutosaveHistoryFile(dir, startedAt, 1234)
	if err != nil {
		t.Fatalf("createAutosaveHistoryFile() pid path error = %v", err)
	}
	if got, want := filepath.Base(path), "history-2026-07-09_08.52.31-pid1234.json"; got != want {
		t.Fatalf("autosave filename = %q, want %q", got, want)
	}

	path, err = createAutosaveHistoryFile(dir, startedAt, 1234)
	if err != nil {
		t.Fatalf("createAutosaveHistoryFile() suffix path error = %v", err)
	}
	if got, want := filepath.Base(path), "history-2026-07-09_08.52.31-pid1234-2.json"; got != want {
		t.Fatalf("autosave filename = %q, want %q", got, want)
	}
}

func TestShouldFlushAutosaveHistoryRequiresTenSamples(t *testing.T) {
	now := time.Date(2026, time.July, 9, 8, 52, 31, 0, time.Local)
	a := &app{sessionPings: minAutosaveHistorySamples - 1, pendingSamples: []historySample{{}}}
	if a.shouldFlushAutosaveHistory(now) {
		t.Fatal("shouldFlushAutosaveHistory returned true before 10 samples")
	}

	a.sessionPings = minAutosaveHistorySamples
	if !a.shouldFlushAutosaveHistory(now) {
		t.Fatal("shouldFlushAutosaveHistory returned false at 10 samples")
	}

	a.pendingSamples = nil
	if a.shouldFlushAutosaveHistory(now) {
		t.Fatal("shouldFlushAutosaveHistory returned true without pending history")
	}
}

func TestShouldFlushAutosaveHistoryUsesIntervalAfterFirstSave(t *testing.T) {
	now := time.Date(2026, time.July, 9, 8, 52, 31, 0, time.Local)
	a := &app{
		sessionPings:    minAutosaveHistorySamples,
		pendingSamples:  []historySample{{}},
		autosavePath:    "history-2026-07-09_08.52.31.json",
		lastHistorySave: now,
	}
	if a.shouldFlushAutosaveHistory(now.Add(autosaveHistoryInterval - time.Second)) {
		t.Fatal("shouldFlushAutosaveHistory returned true before interval elapsed")
	}
	if !a.shouldFlushAutosaveHistory(now.Add(autosaveHistoryInterval)) {
		t.Fatal("shouldFlushAutosaveHistory returned false when interval elapsed")
	}

	a.lastHistorySave = time.Time{}
	if !a.shouldFlushAutosaveHistory(now) {
		t.Fatal("shouldFlushAutosaveHistory returned false before first successful save")
	}
}

func TestParseTargetsPreservesSpecialNames(t *testing.T) {
	got := parseTargets("localhost, gateway; 1.1.1.1")
	want := []string{"localhost", "gateway", "1.1.1.1"}
	if len(got) != len(want) {
		t.Fatalf("parseTargets length = %d, want %d: %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("parseTargets[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestResolveTargetsReplacesSpecialNames(t *testing.T) {
	got := resolveTargets([]string{"localhost", "gateway", "8.8.8.8"}, "192.168.1.1")
	want := []string{"127.0.0.1", "192.168.1.1", "8.8.8.8"}
	if len(got) != len(want) {
		t.Fatalf("resolveTargets length = %d, want %d: %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("resolveTargets[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestResolveTargetsDropsGatewayWhenGatewayMissing(t *testing.T) {
	got := resolveTargets([]string{"gateway", "localhost"}, "")
	want := []string{"127.0.0.1"}
	if len(got) != len(want) {
		t.Fatalf("resolveTargets length = %d, want %d: %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("resolveTargets[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestBucketedYMaxUsesSmallestBucketCovering90Percent(t *testing.T) {
	points := []chartPoint{
		{value: 3, hasValue: true},
		{value: 4, hasValue: true},
		{value: 5, hasValue: true},
		{value: 8, hasValue: true},
		{value: 16, hasValue: true},
		{value: 17, hasValue: true},
		{value: 30, hasValue: true},
		{value: 32, hasValue: true},
		{value: 33, hasValue: true},
		{value: 300, hasValue: true},
	}

	got := bucketedYMax(points, rttYMaxBuckets)
	if got != 64 {
		t.Fatalf("bucketedYMax() = %v, want 64", got)
	}
}

func TestClipLineToRectClipsAtTopBoundary(t *testing.T) {
	rect := walk.Rectangle{X: 10, Y: 20, Width: 100, Height: 50}
	p1, p2, ok := clipLineToRect(rect, 60, 45, 60, -30)
	if !ok {
		t.Fatal("clipLineToRect returned false for line crossing the plot")
	}
	if p1 != (walk.Point{X: 60, Y: 45}) || p2 != (walk.Point{X: 60, Y: 20}) {
		t.Fatalf("clipped line = %v -> %v, want (60,45) -> (60,20)", p1, p2)
	}
}

func TestClipLineToRectRejectsFullyOffPlotLine(t *testing.T) {
	rect := walk.Rectangle{X: 10, Y: 20, Width: 100, Height: 50}
	if _, _, ok := clipLineToRect(rect, 20, 0, 90, 5); ok {
		t.Fatal("clipLineToRect returned true for line fully above the plot")
	}
}

func TestClipLineToRectClipsLeftAndRightBoundaries(t *testing.T) {
	rect := walk.Rectangle{X: 10, Y: 20, Width: 100, Height: 50}
	p1, p2, ok := clipLineToRect(rect, 0, 45, 120, 45)
	if !ok {
		t.Fatal("clipLineToRect returned false for horizontal crossing line")
	}
	if p1 != (walk.Point{X: 10, Y: 45}) || p2 != (walk.Point{X: 110, Y: 45}) {
		t.Fatalf("clipped line = %v -> %v, want (10,45) -> (110,45)", p1, p2)
	}
}

func TestXAxisTicksAimForAtLeastThreeLabelsWhenSpaceAllows(t *testing.T) {
	start := time.Date(2026, time.July, 9, 12, 0, 0, 0, time.Local)
	end := start.Add(2 * time.Minute)
	ticks := xAxisTicks(start, end, 400)
	if len(ticks) < 3 {
		t.Fatalf("len(xAxisTicks) = %d, want at least 3: %v", len(ticks), ticks)
	}
	if len(ticks) > maxXAxisLabels(400) {
		t.Fatalf("len(xAxisTicks) = %d, want at most %d", len(ticks), maxXAxisLabels(400))
	}
}

func TestXAxisTicksAllowFewerThanThreeLabelsInNarrowPlots(t *testing.T) {
	start := time.Date(2026, time.July, 9, 12, 0, 0, 0, time.Local)
	end := start.Add(2 * time.Minute)
	ticks := xAxisTicks(start, end, 100)
	if len(ticks) > maxXAxisLabels(100) {
		t.Fatalf("len(xAxisTicks) = %d, want at most %d", len(ticks), maxXAxisLabels(100))
	}
}

func TestXAxisTicksUseSecondLevelLabels(t *testing.T) {
	start := time.Date(2026, time.July, 9, 12, 0, 3, 0, time.Local)
	end := start.Add(8 * time.Second)
	ticks := xAxisTicks(start, end, 1000)
	if len(ticks) == 0 {
		t.Fatal("xAxisTicks returned no ticks")
	}
	for _, tick := range ticks {
		if len(tick.label) != len("12:00:04") {
			t.Fatalf("tick label = %q, want HH:mm:ss format", tick.label)
		}
	}
}

func TestRealtimeSampleCapacityUsesTenPixelsPerSample(t *testing.T) {
	got := realtimeSampleCapacity(80)
	if got != 8 {
		t.Fatalf("realtimeSampleCapacity() = %d, want 8", got)
	}
	got = realtimeSampleCapacity(100)
	if got != 10 {
		t.Fatalf("realtimeSampleCapacity() = %d, want 10", got)
	}
	if got := realtimeSampleCapacity(0); got != 1 {
		t.Fatalf("zero-width realtimeSampleCapacity() = %d, want 1", got)
	}
}

func TestVisibleRealtimeBarSamplesGroupsByTimestampAndKeepsNewestCapacity(t *testing.T) {
	start := time.Date(2026, time.July, 9, 12, 0, 0, 0, time.Local)
	points := []chartPoint{
		{at: start, groupIndex: 0},
		{at: start, groupIndex: 1},
		{at: start.Add(time.Second), groupIndex: 0},
		{at: start.Add(2 * time.Second), groupIndex: 0},
	}

	all := visibleRealtimeBarSamples(points, 30)
	if len(all) != 3 {
		t.Fatalf("len(visibleRealtimeBarSamples) = %d, want 3", len(all))
	}
	if len(all[0].points) != 2 {
		t.Fatalf("first sample point count = %d, want 2", len(all[0].points))
	}

	got := visibleRealtimeBarSamples(points, 20)
	if len(got) != 2 {
		t.Fatalf("len(visibleRealtimeBarSamples limited) = %d, want 2", len(got))
	}
	if !got[0].at.Equal(start.Add(time.Second)) || !got[1].at.Equal(start.Add(2*time.Second)) {
		t.Fatalf("visible samples kept wrong timestamps: got %v and %v", got[0].at, got[1].at)
	}
}

func TestRealtimeBarTimeRangeAlignsSparseSamplesToRightAnchoredSlots(t *testing.T) {
	start := time.Date(2026, time.July, 9, 12, 0, 0, 0, time.Local)
	samples := []realtimeBarSample{
		{at: start},
		{at: start.Add(time.Second)},
		{at: start.Add(2 * time.Second)},
	}

	gotStart, gotEnd := realtimeBarTimeRange(samples, start.Add(2*time.Second), 100)
	if !gotStart.Equal(start.Add(-7500 * time.Millisecond)) {
		t.Fatalf("range start = %v, want %v", gotStart, start.Add(-7500*time.Millisecond))
	}
	if !gotEnd.Equal(start.Add(2500 * time.Millisecond)) {
		t.Fatalf("range end = %v, want %v", gotEnd, start.Add(2500*time.Millisecond))
	}
	if got := realtimeTimeX(start, gotStart, gotEnd, 100); got != 75 {
		t.Fatalf("first sample x = %v, want 75", got)
	}
	if got := realtimeTimeX(start.Add(2*time.Second), gotStart, gotEnd, 100); got != 95 {
		t.Fatalf("last sample x = %v, want 95", got)
	}
}

func TestRealtimeBarTimeRangeAlignsSingleSampleToRightmostSlot(t *testing.T) {
	at := time.Date(2026, time.July, 9, 12, 0, 0, 0, time.Local)
	gotStart, gotEnd := realtimeBarTimeRange([]realtimeBarSample{{at: at}}, at, 100)
	if got := realtimeTimeX(at, gotStart, gotEnd, 100); got != 95 {
		t.Fatalf("single sample x = %v, want 95", got)
	}
}

func realtimeTimeX(at, start, end time.Time, plotWidth int) int {
	return int(math.Round(at.Sub(start).Seconds() / end.Sub(start).Seconds() * float64(plotWidth)))
}

func TestRealtimeBarSegmentsStackLowToHighAndSkipLoss(t *testing.T) {
	low := walk.RGB(1, 2, 3)
	mid := walk.RGB(4, 5, 6)
	high := walk.RGB(7, 8, 9)
	points := []chartPoint{
		{value: 120, hasValue: true, groupIndex: 2, color: high},
		{lost: true, groupIndex: 1, color: walk.RGB(10, 11, 12)},
		{value: 40, hasValue: true, groupIndex: 0, color: low},
		{value: 90, hasValue: true, groupIndex: 1, color: mid},
	}

	got := realtimeBarSegments(points, 0)
	if len(got) != 3 {
		t.Fatalf("segment count = %d, want 3: %#v", len(got), got)
	}
	want := []realtimeBarSegment{
		{color: low, fromValue: 0, toValue: 40},
		{color: mid, fromValue: 40, toValue: 90},
		{color: high, fromValue: 90, toValue: 120},
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("segment %d = %#v, want %#v", i, got[i], want[i])
		}
	}
}

func TestRealtimeLossMarkerRectStacksFromTopByGroup(t *testing.T) {
	plot := walk.Rectangle{X: 10, Y: 20, Width: 80, Height: 100}
	barX := 30
	group3 := realtimeLossMarkerRect(plot, barX, 0)
	group2 := realtimeLossMarkerRect(plot, barX, 1)
	group1 := realtimeLossMarkerRect(plot, barX, 2)

	if group1.Width != realtimeLossMarkerSize || group1.Height != realtimeLossMarkerSize {
		t.Fatalf("loss marker size = %dx%d, want %dx%d", group1.Width, group1.Height, realtimeLossMarkerSize, realtimeLossMarkerSize)
	}
	if realtimeLossMarkerSize != 10 {
		t.Fatalf("loss marker size = %d, want 10", realtimeLossMarkerSize)
	}
	if realtimeLossMarkerStrokeWidth != 2 {
		t.Fatalf("loss marker stroke width = %d, want 2", realtimeLossMarkerStrokeWidth)
	}
	if group1.X != barX+realtimeBarWidth/2-realtimeLossMarkerSize/2 {
		t.Fatalf("loss marker x = %d, want centered on bar", group1.X)
	}
	if group3.Y != plot.Y {
		t.Fatalf("group 3 loss marker y = %d, want top %d", group3.Y, plot.Y)
	}
	if group2.Y != group3.Y+realtimeLossMarkerSize {
		t.Fatalf("group 2 loss marker y = %d, want %d", group2.Y, group3.Y+realtimeLossMarkerSize)
	}
	if group1.Y != group2.Y+realtimeLossMarkerSize {
		t.Fatalf("group 1 loss marker y = %d, want %d", group1.Y, group2.Y+realtimeLossMarkerSize)
	}
}

func TestRealtimeBarLossMarkersStackOnlyPresentLossesFromTop(t *testing.T) {
	points := []chartPoint{
		{groupIndex: 0, lost: true},
		{groupIndex: 1},
		{groupIndex: 2, lost: true},
	}

	got := realtimeBarLossMarkers(points)
	if len(got) != 2 {
		t.Fatalf("loss marker count = %d, want 2", len(got))
	}
	if got[0].groupIndex != 2 || got[1].groupIndex != 0 {
		t.Fatalf("loss marker order = [%d %d], want [2 0]", got[0].groupIndex, got[1].groupIndex)
	}

	plot := walk.Rectangle{X: 10, Y: 20, Width: 80, Height: 100}
	first := realtimeLossMarkerRect(plot, 30, 0)
	second := realtimeLossMarkerRect(plot, 30, 1)
	if first.Y != plot.Y || second.Y != plot.Y+realtimeLossMarkerSize {
		t.Fatalf("stack y positions = %d/%d, want %d/%d", first.Y, second.Y, plot.Y, plot.Y+realtimeLossMarkerSize)
	}
}

func TestAggregateVisiblePointLimitScalesWithGroupCount(t *testing.T) {
	points := []chartPoint{
		{groupIndex: 0},
		{groupIndex: 1},
		{groupIndex: 2},
		{groupIndex: 0},
	}

	got := aggregateVisiblePointLimit(points, 100)
	want := 3 * (100 / aggregateMinPixelsPerSample)
	if got != want {
		t.Fatalf("aggregateVisiblePointLimit() = %d, want %d", got, want)
	}
}

func TestAggregatePointCapacityUsesMinimumPixelsPerSample(t *testing.T) {
	got := aggregatePointCapacity(1920, aggregateMinPixelsPerSample)
	if got != 320 {
		t.Fatalf("aggregatePointCapacity() = %d, want 320", got)
	}
	if got := aggregatePointCapacity(0, aggregateMinPixelsPerSample); got != 1 {
		t.Fatalf("zero-width aggregatePointCapacity() = %d, want 1", got)
	}
}

func TestMaxAggregatePointsForWidthScalesByGroup(t *testing.T) {
	got := maxAggregatePointsForWidth(3, 1920)
	if got != 960 {
		t.Fatalf("maxAggregatePointsForWidth() = %d, want 960", got)
	}
}

func TestVisibleAggregatePointsKeepsMinimumPixelCapacity(t *testing.T) {
	points := make([]chartPoint, 0, 20)
	for i := 0; i < 20; i++ {
		points = append(points, chartPoint{groupIndex: 0})
	}
	got := visibleAggregatePoints(points, 100)
	wantLen := 100 / aggregateMinPixelsPerSample
	if len(got) != wantLen {
		t.Fatalf("len(visibleAggregatePoints) = %d, want %d", len(got), wantLen)
	}
	if got[0] != points[len(points)-wantLen] {
		t.Fatal("visibleAggregatePoints did not keep the newest points")
	}
}

func TestAggregateChartTimeRangeDoesNotScrollWhenEmpty(t *testing.T) {
	start := time.Date(2026, time.July, 9, 12, 0, 0, 0, time.Local)
	gotStart, gotEnd := aggregateChartTimeRange(nil, 240, 2*time.Minute, start)
	if !gotStart.Equal(start) {
		t.Fatalf("empty aggregate start = %v, want %v", gotStart, start)
	}
	wantEnd := start.Add(2 * time.Minute * time.Duration(240/aggregatePreferredPixelsPerSample-1))
	if !gotEnd.Equal(wantEnd) {
		t.Fatalf("empty aggregate end = %v, want %v", gotEnd, wantEnd)
	}
}

func TestAggregateChartTimeRangeRightAnchorsSparseDataAtPreferredSpacing(t *testing.T) {
	start := time.Date(2026, time.July, 9, 12, 0, 0, 0, time.Local)
	points := []chartPoint{
		{at: start, groupIndex: 0},
		{at: start.Add(2 * time.Minute), groupIndex: 0},
	}
	gotStart, gotEnd := aggregateChartTimeRange(points, 240, 2*time.Minute, time.Time{})
	wantEnd := points[len(points)-1].at
	if !gotEnd.Equal(wantEnd) {
		t.Fatalf("aggregate end = %v, want %v", gotEnd, wantEnd)
	}
	wantStart := wantEnd.Add(-2 * time.Minute * time.Duration(240/aggregatePreferredPixelsPerSample-1))
	if !gotStart.Equal(wantStart) {
		t.Fatalf("aggregate start = %v, want %v", gotStart, wantStart)
	}
}

func TestAggregateChartTimeRangeCompressesUntilMinimumSpacing(t *testing.T) {
	start := time.Date(2026, time.July, 9, 12, 0, 0, 0, time.Local)
	points := make([]chartPoint, 0, 20)
	for i := 0; i < 20; i++ {
		points = append(points, chartPoint{at: start.Add(time.Duration(i) * 2 * time.Minute), groupIndex: 0})
	}
	gotStart, gotEnd := aggregateChartTimeRange(points, 240, 2*time.Minute, time.Time{})
	if !gotStart.Equal(points[0].at) {
		t.Fatalf("aggregate start = %v, want %v", gotStart, points[0].at)
	}
	if !gotEnd.Equal(points[len(points)-1].at) {
		t.Fatalf("aggregate end = %v, want %v", gotEnd, points[len(points)-1].at)
	}
}

func TestRedistributedChartHeightsSharesShrunkSpace(t *testing.T) {
	got := redistributedChartHeights(aggregateChartHeight/3, aggregateChartHeight, true)
	wantTotal := rttChartHeight + 3*aggregateChartHeight
	gotTotal := got[0] + got[1] + got[2] + got[3]
	if gotTotal != wantTotal {
		t.Fatalf("total redistributed height = %d, want %d", gotTotal, wantTotal)
	}
	if got[2] != aggregateChartHeight/3 {
		t.Fatalf("loss height = %d, want %d", got[2], aggregateChartHeight/3)
	}
	if got[0] <= rttChartHeight || got[1] <= aggregateChartHeight || got[3] <= aggregateChartHeight {
		t.Fatalf("unshrunk graphs did not receive saved height: got %v", got)
	}
}

func TestRedistributedChartHeightsLeavesNoInterGraphWasteWhenBothShrink(t *testing.T) {
	got := redistributedChartHeights(aggregateChartHeight/3, aggregateChartHeight/2, true)
	wantTotal := rttChartHeight + 3*aggregateChartHeight
	gotTotal := got[0] + got[1] + got[2] + got[3]
	if gotTotal != wantTotal {
		t.Fatalf("total redistributed height = %d, want %d", gotTotal, wantTotal)
	}
	if got[2] != aggregateChartHeight/3 || got[3] != aggregateChartHeight/2 {
		t.Fatalf("shrunk graph heights changed unexpectedly: got %v", got)
	}
	if got[0] <= rttChartHeight || got[1] <= aggregateChartHeight {
		t.Fatalf("remaining graphs did not receive saved height: got %v", got)
	}
}

func TestRedistributedChartHeightsHidesJitterAndSharesSpace(t *testing.T) {
	got := redistributedChartHeights(aggregateChartHeight, 0, false)
	wantTotal := rttChartHeight + 3*aggregateChartHeight
	gotTotal := got[0] + got[1] + got[2] + got[3]
	if gotTotal != wantTotal {
		t.Fatalf("total redistributed height = %d, want %d", gotTotal, wantTotal)
	}
	if got[3] != 0 {
		t.Fatalf("hidden jitter height = %d, want 0", got[3])
	}
	if got[0] <= rttChartHeight || got[1] <= aggregateChartHeight || got[2] <= aggregateChartHeight {
		t.Fatalf("visible graphs did not receive hidden jitter space: got %v", got)
	}
}

func TestChartWidgetMinSizeDoesNotForceAdaptiveHeight(t *testing.T) {
	got := chartWidgetMinSize()
	if got.Height != 0 {
		t.Fatalf("chart min height = %d, want 0", got.Height)
	}
	if got.Width != 0 {
		t.Fatalf("chart min width = %d, want 0", got.Width)
	}
	if got.Height >= combinedChartHeight(rttChartHeight) {
		t.Fatalf("chart min height follows adaptive height: got %d", got.Height)
	}
}

func TestDefaultUseTypesExcludeRemoteDesktopAndSuperhumanGaming(t *testing.T) {
	got := useTypeSet(defaultUseTypes())
	for _, name := range []string{"Browsing & Email", "Audio Calls", "Video Calls", "Online Gaming"} {
		if !got[name] {
			t.Fatalf("default uses missing %q: got %v", name, defaultUseTypes())
		}
	}
	for _, name := range []string{"Remote Desktop", "Superhuman Gaming"} {
		if got[name] {
			t.Fatalf("default uses unexpectedly include %q: got %v", name, defaultUseTypes())
		}
	}
}

func TestUseProfileThresholds(t *testing.T) {
	tests := []struct {
		name   string
		rtt    [3]float64
		loss   [3]float64
		jitter [3]float64
	}{
		{name: "Browsing & Email", rtt: [3]float64{150, 300, 600}, loss: [3]float64{0.5, 2, 5}, jitter: [3]float64{600, 600, 600}},
		{name: "Remote Desktop", rtt: [3]float64{100, 150, 220}, loss: [3]float64{1, 2, 3}, jitter: [3]float64{15, 30, 50}},
		{name: "Audio Calls", rtt: [3]float64{100, 150, 250}, loss: [3]float64{1, 2, 3}, jitter: [3]float64{20, 30, 50}},
		{name: "Video Calls", rtt: [3]float64{100, 150, 250}, loss: [3]float64{1, 2, 3}, jitter: [3]float64{20, 30, 50}},
		{name: "Online Gaming", rtt: [3]float64{50, 80, 120}, loss: [3]float64{0.5, 1, 2}, jitter: [3]float64{10, 20, 30}},
		{name: "Superhuman Gaming", rtt: [3]float64{20, 35, 60}, loss: [3]float64{0.1, 0.5, 1}, jitter: [3]float64{5, 10, 20}},
	}
	for _, tt := range tests {
		got, ok := profileFor(tt.name)
		if !ok {
			t.Fatalf("profileFor(%q) returned false", tt.name)
		}
		if got.RTT != tt.rtt {
			t.Fatalf("%s RTT thresholds = %v, want %v", tt.name, got.RTT, tt.rtt)
		}
		if got.Loss != tt.loss {
			t.Fatalf("%s loss thresholds = %v, want %v", tt.name, got.Loss, tt.loss)
		}
		if got.Jitter != tt.jitter {
			t.Fatalf("%s jitter thresholds = %v, want %v", tt.name, got.Jitter, tt.jitter)
		}
	}
}

func TestProfileForUsesUsesMostDemandingThresholds(t *testing.T) {
	got := profileForUses([]string{"Browsing & Email", "Online Gaming", "Video Calls"})
	if got.RTT != [3]float64{50, 80, 120} {
		t.Fatalf("RTT thresholds = %v, want [50 80 120]", got.RTT)
	}
	if got.Loss != [3]float64{0.5, 1, 2} {
		t.Fatalf("loss thresholds = %v, want [0.5 1 2]", got.Loss)
	}
	if got.Jitter != [3]float64{10, 20, 30} {
		t.Fatalf("jitter thresholds = %v, want [10 20 30]", got.Jitter)
	}
}

func TestUsesShowJitterOnlyForAudioOrVideoCalls(t *testing.T) {
	if usesShowJitter([]string{"Browsing & Email", "Online Gaming"}) {
		t.Fatal("usesShowJitter returned true without audio or video calls")
	}
	if !usesShowJitter([]string{"Audio Calls"}) {
		t.Fatal("usesShowJitter returned false for Audio Calls")
	}
	if !usesShowJitter([]string{"Video Calls"}) {
		t.Fatal("usesShowJitter returned false for Video Calls")
	}
}

func TestNormalizeUseTypesRenamesLowLatencyGaming(t *testing.T) {
	got := normalizeUseTypes([]string{"low latency gaming"}, "")
	if len(got) != 1 || got[0] != "Superhuman Gaming" {
		t.Fatalf("normalizeUseTypes(low latency gaming) = %v, want [Superhuman Gaming]", got)
	}
}

func TestNormalizeUseTypesRenamesEmailBrowsing(t *testing.T) {
	got := normalizeUseTypes([]string{"email & browsing"}, "")
	if len(got) != 1 || got[0] != "Browsing & Email" {
		t.Fatalf("normalizeUseTypes(email & browsing) = %v, want [Browsing & Email]", got)
	}
}

func TestSummarizeConnectionIssuesGood(t *testing.T) {
	got, severity := summarizeConnectionIssues(nil)
	if got != "So far, the connection looks good." {
		t.Fatalf("summary = %q", got)
	}
	if severity != 0 {
		t.Fatalf("severity = %d, want 0", severity)
	}
}

func TestSummarizeConnectionIssuesSingleIssue(t *testing.T) {
	at := issueTestTime(12, 34)
	got, severity := summarizeConnectionIssues([]connectionIssue{{at: at, severity: 1}})
	want := "The connection has had a minor issue at 12:34."
	if got != want {
		t.Fatalf("summary = %q, want %q", got, want)
	}
	if severity != 1 {
		t.Fatalf("severity = %d, want 1", severity)
	}
}

func TestSummarizeConnectionIssuesSomeSameSeverity(t *testing.T) {
	got, severity := summarizeConnectionIssues([]connectionIssue{
		{at: issueTestTime(12, 10), severity: 2},
		{at: issueTestTime(12, 15), severity: 2},
	})
	want := "The connection has had some noticeable issues (between 12:10-12:15)."
	if got != want {
		t.Fatalf("summary = %q, want %q", got, want)
	}
	if severity != 2 {
		t.Fatalf("severity = %d, want 2", severity)
	}
}

func TestSummarizeConnectionIssuesSeveralSameSeverity(t *testing.T) {
	got, severity := summarizeConnectionIssues([]connectionIssue{
		{at: issueTestTime(12, 10), severity: 3},
		{at: issueTestTime(12, 11), severity: 3},
		{at: issueTestTime(12, 13), severity: 3},
		{at: issueTestTime(12, 15), severity: 3},
	})
	want := "The connection has had several serious issues (between 12:10-12:15)."
	if got != want {
		t.Fatalf("summary = %q, want %q", got, want)
	}
	if severity != 3 {
		t.Fatalf("severity = %d, want 3", severity)
	}
}

func TestSummarizeConnectionIssuesCombinesSeverities(t *testing.T) {
	got, severity := summarizeConnectionIssues([]connectionIssue{
		{at: issueTestTime(12, 34), severity: 1},
		{at: issueTestTime(12, 18), severity: 2},
		{at: issueTestTime(12, 20), severity: 2},
		{at: issueTestTime(12, 10), severity: 3},
		{at: issueTestTime(12, 11), severity: 3},
		{at: issueTestTime(12, 13), severity: 3},
		{at: issueTestTime(12, 15), severity: 3},
	})
	want := "The connection has had: a minor issue at 12:34, some noticeable issues (last at 12:20), several serious issues (between 12:10-12:15)."
	if got != want {
		t.Fatalf("summary = %q, want %q", got, want)
	}
	if severity != 3 {
		t.Fatalf("severity = %d, want 3", severity)
	}
}

func TestConnectionIssuesPreferAggregatesOverRealtime(t *testing.T) {
	at := issueTestTime(12, 34)
	a := &app{
		settings: savedConfig{UseTypes: []string{"Online Gaming"}},
		samples: []sampleEvent{
			{at: at, rtt: 300, replied: true, targetLabel: "Internet"},
		},
		aggregates: []aggregatePoint{
			{at: at, p95: 20, loss: 0, jitterP95: 0},
		},
		historyViewEnd: at,
	}
	got, severity := summarizeConnectionIssues(a.connectionIssues())
	if got != "So far, the connection looks good." {
		t.Fatalf("summary = %q", got)
	}
	if severity != 0 {
		t.Fatalf("severity = %d, want 0", severity)
	}
}

func TestConnectionIssuesFallBackToRealtimeBeforeAggregates(t *testing.T) {
	at := issueTestTime(12, 34)
	a := &app{
		settings: savedConfig{UseTypes: []string{"Online Gaming"}},
		samples: []sampleEvent{
			{at: at, rtt: 300, replied: true, targetLabel: "Internet"},
		},
		historyViewEnd: at,
	}
	got, severity := summarizeConnectionIssues(a.connectionIssues())
	want := "The connection has had a serious issue at 12:34."
	if got != want {
		t.Fatalf("summary = %q, want %q", got, want)
	}
	if severity != 3 {
		t.Fatalf("severity = %d, want 3", severity)
	}
}

func TestConnectionIssuesUseVisibleAggregateGraphs(t *testing.T) {
	at := issueTestTime(12, 34)
	hiddenJitter := &app{
		settings: savedConfig{UseTypes: []string{"Online Gaming"}},
		aggregates: []aggregatePoint{
			{at: at, p95: 20, loss: 0, jitterP95: 500},
		},
	}
	got, severity := summarizeConnectionIssues(hiddenJitter.connectionIssues())
	if got != "So far, the connection looks good." {
		t.Fatalf("hidden jitter summary = %q", got)
	}
	if severity != 0 {
		t.Fatalf("hidden jitter severity = %d, want 0", severity)
	}

	visibleJitter := &app{
		settings: savedConfig{UseTypes: []string{"Audio Calls"}},
		aggregates: []aggregatePoint{
			{at: at, p95: 20, loss: 0, jitterP95: 500},
		},
	}
	got, severity = summarizeConnectionIssues(visibleJitter.connectionIssues())
	want := "The connection has had a serious issue at 12:34."
	if got != want {
		t.Fatalf("visible jitter summary = %q, want %q", got, want)
	}
	if severity != 3 {
		t.Fatalf("visible jitter severity = %d, want 3", severity)
	}
}

func issueTestTime(hour, minute int) time.Time {
	return time.Date(2026, time.July, 6, hour, minute, 0, 0, time.Local)
}
