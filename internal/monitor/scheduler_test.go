package monitor

import (
	"context"
	"net/netip"
	"runtime"
	"testing"
	"time"

	"pingaro/internal/probe"
)

func TestSchedulerEmitsFixedCadenceWhileRequestsAreOutstanding(t *testing.T) {
	start := time.Date(2026, time.July, 11, 10, 0, 0, 0, time.UTC)
	clock := newFakeClock(start)
	prober := newControlledProber()
	results := make(chan BatchResult, 10)
	scheduler := NewScheduler(SchedulerConfig{
		Clock:                clock,
		Prober:               prober,
		SessionID:            1,
		Interval:             50 * time.Millisecond,
		ReplyTimeout:         500 * time.Millisecond,
		PerTargetOutstanding: 20,
		GlobalOutstanding:    20,
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go scheduler.Run(ctx, []Group{{ID: 0, Name: "Internet", Targets: []string{"target"}}}, func(result BatchResult) {
		results <- result
	})

	for i := 0; i < 4; i++ {
		req := prober.waitRequest(t)
		wantSentAt := start.Add(time.Duration(i) * 50 * time.Millisecond)
		if !req.SentAt.Equal(wantSentAt) {
			t.Fatalf("request %d SentAt = %v, want %v", i+1, req.SentAt, wantSentAt)
		}
		if i < 3 {
			clock.Advance(50 * time.Millisecond)
		}
	}
	assertNoResult(t, results)
}

func TestSchedulerDeadlineBoundaryRule(t *testing.T) {
	req := probe.Request{ID: 1, Target: "target"}
	deadline := time.Date(2026, time.July, 11, 10, 0, 0, 500*int(time.Millisecond), time.UTC)
	reply := probe.NewReply(req, netip.MustParseAddr("192.0.2.10"), 500*time.Millisecond)
	if !isOnTimeReply(reply, deadline, deadline, 500*time.Millisecond) {
		t.Fatal("reply completed exactly at the deadline should be on time")
	}
	if isOnTimeReply(reply, deadline.Add(time.Nanosecond), deadline, 500*time.Millisecond) {
		t.Fatal("reply completed after the deadline should be late")
	}
	if isOnTimeReply(probe.NewReply(req, netip.MustParseAddr("192.0.2.10"), 501*time.Millisecond), deadline, deadline, 500*time.Millisecond) {
		t.Fatal("reply RTT above the timeout should be late")
	}
}

func TestSchedulerReplyBeforeDeadlineSucceeds(t *testing.T) {
	start := time.Date(2026, time.July, 11, 10, 0, 0, 0, time.UTC)
	clock := newFakeClock(start)
	prober := newControlledProber()
	results := make(chan BatchResult, 10)
	scheduler := NewScheduler(SchedulerConfig{
		Clock:                clock,
		Prober:               prober,
		SessionID:            1,
		Interval:             time.Minute,
		ReplyTimeout:         500 * time.Millisecond,
		PerTargetOutstanding: 10,
		GlobalOutstanding:    10,
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go scheduler.Run(ctx, []Group{{ID: 0, Name: "Internet", Targets: []string{"target"}}}, func(result BatchResult) {
		results <- result
	})

	req := prober.waitRequest(t)
	clock.Advance(499 * time.Millisecond)
	prober.complete(req.ID, probe.NewReply(req, netip.MustParseAddr("192.0.2.10"), 123*time.Millisecond))
	got := waitResult(t, results)
	if got.Kind != probe.OutcomeReply || got.RTT != 123*time.Millisecond {
		t.Fatalf("boundary result = kind %v rtt %v, want reply 123ms", got.Kind, got.RTT)
	}
}

func TestSchedulerLateReplyDoesNotModifyFinalizedTimeout(t *testing.T) {
	start := time.Date(2026, time.July, 11, 10, 0, 0, 0, time.UTC)
	clock := newFakeClock(start)
	prober := newControlledProber()
	results := make(chan BatchResult, 10)
	scheduler := NewScheduler(SchedulerConfig{
		Clock:                clock,
		Prober:               prober,
		SessionID:            1,
		Interval:             time.Minute,
		ReplyTimeout:         500 * time.Millisecond,
		PerTargetOutstanding: 10,
		GlobalOutstanding:    10,
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go scheduler.Run(ctx, []Group{{ID: 0, Name: "Internet", Targets: []string{"target"}}}, func(result BatchResult) {
		results <- result
	})

	req := prober.waitRequest(t)
	clock.Advance(501 * time.Millisecond)
	got := waitResult(t, results)
	if got.Kind != probe.OutcomeTimeout {
		t.Fatalf("deadline result kind = %v, want timeout", got.Kind)
	}

	prober.complete(req.ID, probe.NewReply(req, netip.MustParseAddr("192.0.2.10"), 100*time.Millisecond))
	assertNoResult(t, results)
}

func TestSchedulerOutstandingLimitEmitsNotSentAndReleasesCapacity(t *testing.T) {
	start := time.Date(2026, time.July, 11, 10, 0, 0, 0, time.UTC)
	clock := newFakeClock(start)
	prober := newControlledProber()
	results := make(chan BatchResult, 10)
	scheduler := NewScheduler(SchedulerConfig{
		Clock:                clock,
		Prober:               prober,
		SessionID:            1,
		Interval:             50 * time.Millisecond,
		ReplyTimeout:         500 * time.Millisecond,
		PerTargetOutstanding: 1,
		GlobalOutstanding:    10,
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go scheduler.Run(ctx, []Group{{ID: 0, Name: "Internet", Targets: []string{"target"}}}, func(result BatchResult) {
		results <- result
	})

	first := prober.waitRequest(t)
	clock.Advance(50 * time.Millisecond)
	notSent := waitResult(t, results)
	if notSent.Kind != probe.OutcomeNotSent || notSent.Kind.CountsAsNetworkLoss() {
		t.Fatalf("limit result = kind %v networkLoss %v, want not-sent non-loss", notSent.Kind, notSent.Kind.CountsAsNetworkLoss())
	}

	prober.complete(first.ID, probe.NewReply(first, netip.MustParseAddr("192.0.2.10"), 20*time.Millisecond))
	reply := waitResult(t, results)
	if reply.Kind != probe.OutcomeReply {
		t.Fatalf("first request result = %v, want reply", reply.Kind)
	}

	clock.Advance(50 * time.Millisecond)
	next := prober.waitRequest(t)
	if !next.SentAt.Equal(start.Add(100 * time.Millisecond)) {
		t.Fatalf("next sent at %v, want %v", next.SentAt, start.Add(100*time.Millisecond))
	}
}

func TestSchedulerSkipsMissedSlotsWithoutBurst(t *testing.T) {
	start := time.Date(2026, time.July, 11, 10, 0, 0, 0, time.UTC)
	clock := newFakeClock(start)
	prober := newControlledProber()
	scheduler := NewScheduler(SchedulerConfig{
		Clock:                clock,
		Prober:               prober,
		SessionID:            1,
		Interval:             50 * time.Millisecond,
		ReplyTimeout:         500 * time.Millisecond,
		PerTargetOutstanding: 100,
		GlobalOutstanding:    100,
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go scheduler.Run(ctx, []Group{{ID: 0, Name: "Internet", Targets: []string{"target"}}}, func(BatchResult) {})

	_ = prober.waitRequest(t)
	clock.Advance(3 * time.Second)
	waitForFutureTimer(t, clock)
	assertNoRequest(t, prober.requests)

	clock.Advance(50 * time.Millisecond)
	next := prober.waitRequest(t)
	minWant := start.Add(3050 * time.Millisecond)
	if next.SentAt.Before(minWant) {
		t.Fatalf("resumed SentAt = %v, want no earlier than %v", next.SentAt, minWant)
	}
}

func TestSchedulerIgnoresOutcomesFromAnotherSession(t *testing.T) {
	start := time.Date(2026, time.July, 11, 10, 0, 0, 0, time.UTC)
	clock := newFakeClock(start)
	prober := newControlledProber()
	results := make(chan BatchResult, 10)
	scheduler := NewScheduler(SchedulerConfig{
		Clock:                clock,
		Prober:               prober,
		SessionID:            2,
		Interval:             time.Minute,
		ReplyTimeout:         500 * time.Millisecond,
		PerTargetOutstanding: 10,
		GlobalOutstanding:    10,
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go scheduler.Run(ctx, []Group{{ID: 0, Name: "Internet", Targets: []string{"target"}}}, func(result BatchResult) {
		results <- result
	})

	req := prober.waitRequest(t)
	oldReq := req
	oldReq.SessionID = 1
	prober.complete(req.ID, probe.NewReply(oldReq, netip.MustParseAddr("192.0.2.10"), 20*time.Millisecond))
	assertNoResult(t, results)
}

type controlledProber struct {
	requests    chan probe.Request
	completions chan controlledCompletion
}

type controlledCompletion struct {
	requestID uint64
	outcome   probe.Outcome
}

func newControlledProber() *controlledProber {
	return &controlledProber{
		requests:    make(chan probe.Request, 100),
		completions: make(chan controlledCompletion, 100),
	}
}

func (p *controlledProber) Probe(ctx context.Context, req probe.Request) probe.Outcome {
	p.requests <- req
	for {
		select {
		case completion := <-p.completions:
			if completion.requestID == req.ID {
				return completion.outcome
			}
			p.completions <- completion
		case <-ctx.Done():
			return probe.NewCancelled(req).WithDetail("context cancelled")
		}
	}
}

func (p *controlledProber) waitRequest(t *testing.T) probe.Request {
	t.Helper()
	select {
	case req := <-p.requests:
		return req
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for request")
		return probe.Request{}
	}
}

func (p *controlledProber) complete(requestID uint64, outcome probe.Outcome) {
	p.completions <- controlledCompletion{requestID: requestID, outcome: outcome}
}

func waitResult(t *testing.T, results <-chan BatchResult) BatchResult {
	t.Helper()
	select {
	case result := <-results:
		return result
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for result")
		return BatchResult{}
	}
}

func assertNoResult(t *testing.T, results <-chan BatchResult) {
	t.Helper()
	select {
	case result := <-results:
		t.Fatalf("unexpected result: %+v", result)
	default:
	}
}

func assertNoRequest(t *testing.T, requests <-chan probe.Request) {
	t.Helper()
	select {
	case req := <-requests:
		t.Fatalf("unexpected request: %+v", req)
	default:
	}
}

func waitForFutureTimer(t *testing.T, clock *fakeClock) {
	t.Helper()
	for i := 0; i < 100; i++ {
		for _, timer := range clock.timers {
			if timer.active && timer.deadline.After(clock.Now()) {
				return
			}
		}
		runtime.Gosched()
	}
	t.Fatal("timed out waiting for scheduler to reset a future timer")
}
