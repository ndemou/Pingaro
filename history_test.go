package main

import (
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
	got := defaultGroups("192.168.1.1")
	if len(got) != 2 {
		t.Fatalf("len(defaultGroups) = %d, want 2", len(got))
	}
	if got[0].Name != "Gateway" || got[0].Targets != "192.168.1.1" {
		t.Fatalf("group 1 = %+v, want Gateway 192.168.1.1", got[0])
	}
	if got[1].Name != "Internet" || got[1].Targets != defaultInternetTargets {
		t.Fatalf("group 2 = %+v, want Internet defaults", got[1])
	}
}

func TestDefaultGroupsUseInternetWhenGatewayMissing(t *testing.T) {
	got := defaultGroups("")
	if len(got) != 1 {
		t.Fatalf("len(defaultGroups) = %d, want 1", len(got))
	}
	if got[0].Name != "Internet" || got[0].Targets != defaultInternetTargets {
		t.Fatalf("group 1 = %+v, want Internet defaults", got[0])
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
	for _, name := range []string{"email & browsing", "audio calls", "video calls", "online gaming"} {
		if !got[name] {
			t.Fatalf("default uses missing %q: got %v", name, defaultUseTypes())
		}
	}
	for _, name := range []string{"remote desktop", "Superhuman Gaming"} {
		if got[name] {
			t.Fatalf("default uses unexpectedly include %q: got %v", name, defaultUseTypes())
		}
	}
}

func TestProfileForUsesUsesMostDemandingThresholds(t *testing.T) {
	got := profileForUses([]string{"email & browsing", "online gaming", "video calls"})
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
	if usesShowJitter([]string{"email & browsing", "online gaming"}) {
		t.Fatal("usesShowJitter returned true without audio or video calls")
	}
	if !usesShowJitter([]string{"audio calls"}) {
		t.Fatal("usesShowJitter returned false for audio calls")
	}
	if !usesShowJitter([]string{"video calls"}) {
		t.Fatal("usesShowJitter returned false for video calls")
	}
}

func TestNormalizeUseTypesRenamesLowLatencyGaming(t *testing.T) {
	got := normalizeUseTypes([]string{"low latency gaming"}, "")
	if len(got) != 1 || got[0] != "Superhuman Gaming" {
		t.Fatalf("normalizeUseTypes(low latency gaming) = %v, want [Superhuman Gaming]", got)
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
		settings: savedConfig{UseTypes: []string{"online gaming"}},
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
		settings: savedConfig{UseTypes: []string{"online gaming"}},
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
		settings: savedConfig{UseTypes: []string{"online gaming"}},
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
		settings: savedConfig{UseTypes: []string{"audio calls"}},
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
