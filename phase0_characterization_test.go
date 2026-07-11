package main

import (
	"context"
	"math"
	"strings"
	"testing"
	"time"

	"pingaro/internal/probe"
)

func TestPingBatchSucceedsWhenOneTargetReplies(t *testing.T) {
	at := time.Date(2026, time.July, 11, 10, 0, 0, 0, time.Local)
	prober := fakePingProber(map[string]pingResult{
		"1.1.1.1": {sentAt: at, rtt: lostRTT, destination: "1.1.1.1", status: "TimeOut", warning: "request timed out"},
		"8.8.8.8": {sentAt: at, rtt: 42, destination: "8.8.8.8", status: "Success"},
	})

	got := pingBatchWithProber(context.Background(), []string{"1.1.1.1", "8.8.8.8"}, "Internet", at, prober)
	if got.status != "Success" {
		t.Fatalf("status = %q, want Success", got.status)
	}
	if got.rtt != 42 {
		t.Fatalf("rtt = %d, want 42", got.rtt)
	}
	if got.destination != "Internet" {
		t.Fatalf("destination = %q, want group destination", got.destination)
	}
}

func TestPingBatchSelectsMinimumRTTWhenSeveralTargetsReply(t *testing.T) {
	at := time.Date(2026, time.July, 11, 10, 0, 0, 0, time.Local)
	prober := fakePingProber(map[string]pingResult{
		"fast": {sentAt: at, rtt: 18, destination: "fast", status: "Success"},
		"mid":  {sentAt: at, rtt: 40, destination: "mid", status: "Success"},
		"slow": {sentAt: at, rtt: 90, destination: "slow", status: "Success"},
	})

	got := pingBatchWithProber(context.Background(), []string{"slow", "fast", "mid"}, "Internet", at, prober)
	if got.status != "Success" || got.rtt != 18 {
		t.Fatalf("result = %+v, want Success with min RTT 18", got)
	}
}

func TestPingBatchAllFailuresAreLostAndKeepDiagnostics(t *testing.T) {
	at := time.Date(2026, time.July, 11, 10, 0, 0, 0, time.Local)
	prober := fakePingProber(map[string]pingResult{
		"timeout":     {sentAt: at, rtt: lostRTT, destination: "timeout", status: "TimeOut", warning: "request timed out"},
		"unreachable": {sentAt: at, rtt: lostRTT, destination: "unreachable", status: "PingFailed", warning: "destination host unreachable"},
	})

	got := pingBatchWithProber(context.Background(), []string{"timeout", "unreachable"}, "Internet", at, prober)
	if got.status != "failure" {
		t.Fatalf("status = %q, want failure", got.status)
	}
	if got.rtt != lostRTT {
		t.Fatalf("rtt = %d, want lostRTT", got.rtt)
	}
	for _, want := range []string{"timeout: request timed out", "unreachable: destination host unreachable"} {
		if !strings.Contains(got.warning, want) {
			t.Fatalf("warning = %q, want it to contain %q", got.warning, want)
		}
	}
}

func TestPingBatchRetainsCancelledAndLocalFailureDiagnostics(t *testing.T) {
	at := time.Date(2026, time.July, 11, 10, 0, 0, 0, time.Local)
	prober := fakePingProber(map[string]pingResult{
		"cancelled": {sentAt: at, rtt: lostRTT, destination: "cancelled", status: "Cancelled", warning: "context cancelled"},
		"local":     {sentAt: at, rtt: lostRTT, destination: "local", status: "PingFailed", warning: "local process failure"},
	})

	got := pingBatchWithProber(context.Background(), []string{"cancelled", "local"}, "Internet", at, prober)
	for _, want := range []string{"cancelled: context cancelled", "local: local process failure"} {
		if !strings.Contains(got.warning, want) {
			t.Fatalf("warning = %q, want it to contain %q", got.warning, want)
		}
	}
}

func TestStatsIntsFreezesExistingPercentileBehavior(t *testing.T) {
	got := statsInts([]int{50, 10, 40, 20, 30})
	want := stats{min: 10, p5: 10, median: 30, p95: 40, max: 50}
	if got != want {
		t.Fatalf("statsInts() = %+v, want %+v", got, want)
	}
}

func TestP95JitterSkipsLostSamplesAndUsesExistingPercentile(t *testing.T) {
	got := p95Jitter([]int{100, 140, lostRTT, 200, 260})
	if got != 20 {
		t.Fatalf("p95Jitter() = %v, want 20", got)
	}
}

func TestStreamStateAcceptTracksLossMinMaxAndAggregationBoundary(t *testing.T) {
	at := time.Date(2026, time.July, 11, 10, 0, 0, 0, time.Local)
	state := streamState{
		targetLabel:   "Internet",
		aggSeconds:    3,
		pingsPerBatch: 1,
		minRTT:        math.MaxInt,
	}

	first := state.accept(pingResult{sentAt: at, rtt: 100, status: "Success"})
	if first.total != 1 || first.lostTotal != 0 || first.minRTT != 100 || first.maxRTT != 100 || first.p95 != 100 || first.aggregate != nil {
		t.Fatalf("first event = %+v", first)
	}

	lost := state.accept(pingResult{sentAt: at.Add(time.Second), rtt: lostRTT, status: "TimeOut"})
	if !lost.lost || lost.rtt != lostRTT || lost.total != 2 || lost.lostTotal != 1 || lost.lossPercent != 50 || lost.windowLoss != 50 {
		t.Fatalf("lost event = %+v", lost)
	}
	if lost.minRTT != 100 || lost.maxRTT != 100 {
		t.Fatalf("lost event min/max = %d/%d, want 100/100", lost.minRTT, lost.maxRTT)
	}

	state.accept(pingResult{sentAt: at.Add(2 * time.Second), rtt: 200, status: "Success"})
	aggregateEvent := state.accept(pingResult{sentAt: at.Add(3 * time.Second), rtt: 300, status: "Success"})
	if aggregateEvent.aggregate == nil {
		t.Fatal("aggregate event missing aggregate at boundary")
	}
	if aggregateEvent.p95 != 200 || aggregateEvent.aggregate.p95 != 200 {
		t.Fatalf("aggregate p95 = %v / %v, want 200", aggregateEvent.p95, aggregateEvent.aggregate.p95)
	}
	if aggregateEvent.jitterP95 != 50 || aggregateEvent.aggregate.jitterP95 != 50 {
		t.Fatalf("aggregate jitter = %v / %v, want 50", aggregateEvent.jitterP95, aggregateEvent.aggregate.jitterP95)
	}
	if !closeFloat(aggregateEvent.windowLoss, 100.0/3.0) {
		t.Fatalf("windowLoss = %v, want %v", aggregateEvent.windowLoss, 100.0/3.0)
	}
}

func TestStreamStateAllLossesHaveZeroRTTStatsAndFullLoss(t *testing.T) {
	at := time.Date(2026, time.July, 11, 10, 0, 0, 0, time.Local)
	state := streamState{targetLabel: "Internet", aggSeconds: 60, pingsPerBatch: 1, minRTT: math.MaxInt}

	state.accept(pingResult{sentAt: at, rtt: lostRTT, status: "TimeOut"})
	got := state.accept(pingResult{sentAt: at.Add(time.Second), rtt: lostRTT, status: "TimeOut"})
	if got.minRTT != 0 || got.maxRTT != 0 || got.p95 != 0 || got.jitterP95 != 0 {
		t.Fatalf("all-loss stats = min %d max %d p95 %v jitter %v, want zero RTT stats", got.minRTT, got.maxRTT, got.p95, got.jitterP95)
	}
	if got.lossPercent != 100 || got.windowLoss != 100 {
		t.Fatalf("all-loss percentages = total %v window %v, want 100/100", got.lossPercent, got.windowLoss)
	}
}

func TestStreamStateNoLossesTrackExistingP95AndJitter(t *testing.T) {
	at := time.Date(2026, time.July, 11, 10, 0, 0, 0, time.Local)
	state := streamState{targetLabel: "Internet", aggSeconds: 60, pingsPerBatch: 1, minRTT: math.MaxInt}

	state.accept(pingResult{sentAt: at, rtt: 50, status: "Success"})
	state.accept(pingResult{sentAt: at.Add(time.Second), rtt: 100, status: "Success"})
	got := state.accept(pingResult{sentAt: at.Add(2 * time.Second), rtt: 150, status: "Success"})
	if got.minRTT != 50 || got.maxRTT != 150 || got.p95 != 100 || got.jitterP95 != 25 {
		t.Fatalf("no-loss stats = min %d max %d p95 %v jitter %v, want 50/150/100/25", got.minRTT, got.maxRTT, got.p95, got.jitterP95)
	}
	if got.lossPercent != 0 || got.windowLoss != 0 {
		t.Fatalf("no-loss percentages = total %v window %v, want zero", got.lossPercent, got.windowLoss)
	}
}

type fakePingProber map[string]pingResult

func (p fakePingProber) Probe(ctx context.Context, req probe.Request) probe.Outcome {
	result, ok := p[req.Target]
	if !ok {
		result = pingResult{rtt: lostRTT, destination: req.Target, status: "TimeOut", warning: "missing fake result"}
	}
	result.sentAt = req.SentAt
	return probe.NewOutcome(req, result.rtt, result.destination, result.status, result.warning)
}

func closeFloat(a, b float64) bool {
	return math.Abs(a-b) < 1e-9
}
