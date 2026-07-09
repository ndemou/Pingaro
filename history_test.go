package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/lxn/walk"
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
		walk.RGB(0, 0, 0),
		walk.RGB(133, 0, 135),
		walk.RGB(40, 124, 39),
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
		{value: 3},
		{value: 4},
		{value: 5},
		{value: 8},
		{value: 16},
		{value: 17},
		{value: 30},
		{value: 32},
		{value: 33},
		{value: 300},
	}

	got := bucketedYMax(points, rttYMaxBuckets, -1)
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

func TestAggregateVisiblePointLimitScalesWithGroupCount(t *testing.T) {
	points := []chartPoint{
		{groupIndex: 0},
		{groupIndex: 1},
		{groupIndex: 2},
		{groupIndex: 0},
	}

	got := aggregateVisiblePointLimit(points, 100)
	if got != 300 {
		t.Fatalf("aggregateVisiblePointLimit() = %d, want 300", got)
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

func TestProfileForUsesUsesMostDemandingThresholds(t *testing.T) {
	got := profileForUses([]string{"Browsing & Email", "Online Gaming", "Video Calls"})
	if got.RTT != [3]float64{80, 140, 220} {
		t.Fatalf("RTT thresholds = %v, want [80 140 220]", got.RTT)
	}
	if got.Loss != [3]float64{0.5, 1.5, 4} {
		t.Fatalf("loss thresholds = %v, want [0.5 1.5 4]", got.Loss)
	}
	if got.Jitter != [3]float64{15, 30, 60} {
		t.Fatalf("jitter thresholds = %v, want [15 30 60]", got.Jitter)
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
			{at: at, rtt: 300, targetLabel: "Internet"},
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
			{at: at, rtt: 300, targetLabel: "Internet"},
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
