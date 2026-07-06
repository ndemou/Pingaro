package main

import "testing"

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
