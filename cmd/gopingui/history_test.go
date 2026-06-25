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
