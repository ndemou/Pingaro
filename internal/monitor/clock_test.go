package monitor

import (
	"testing"
	"time"
)

func TestFakeClockTimersFireWhenAdvanced(t *testing.T) {
	start := time.Date(2026, time.July, 11, 10, 0, 0, 0, time.UTC)
	clock := newFakeClock(start)
	timer := clock.NewTimer(50 * time.Millisecond)

	clock.Advance(49 * time.Millisecond)
	select {
	case got := <-timer.C():
		t.Fatalf("timer fired early at %v", got)
	default:
	}

	clock.Advance(time.Millisecond)
	select {
	case got := <-timer.C():
		if !got.Equal(start.Add(50 * time.Millisecond)) {
			t.Fatalf("timer fired at %v, want %v", got, start.Add(50*time.Millisecond))
		}
	default:
		t.Fatal("timer did not fire after deadline")
	}
}

func TestFakeClockTimerReset(t *testing.T) {
	start := time.Date(2026, time.July, 11, 10, 0, 0, 0, time.UTC)
	clock := newFakeClock(start)
	timer := clock.NewTimer(50 * time.Millisecond)

	if !timer.Reset(100 * time.Millisecond) {
		t.Fatal("Reset returned false for active timer")
	}
	clock.Advance(50 * time.Millisecond)
	select {
	case got := <-timer.C():
		t.Fatalf("timer fired at old deadline: %v", got)
	default:
	}

	clock.Advance(50 * time.Millisecond)
	select {
	case got := <-timer.C():
		if !got.Equal(start.Add(100 * time.Millisecond)) {
			t.Fatalf("timer fired at %v, want %v", got, start.Add(100*time.Millisecond))
		}
	default:
		t.Fatal("timer did not fire at reset deadline")
	}
}

type fakeClock struct {
	now    time.Time
	timers []*fakeTimer
}

func newFakeClock(now time.Time) *fakeClock {
	return &fakeClock{now: now}
}

func (c *fakeClock) Now() time.Time {
	return c.now
}

func (c *fakeClock) NewTimer(d time.Duration) Timer {
	timer := &fakeTimer{
		clock:    c,
		deadline: c.now.Add(d),
		active:   true,
		ch:       make(chan time.Time, 1),
	}
	c.timers = append(c.timers, timer)
	return timer
}

func (c *fakeClock) Advance(d time.Duration) {
	c.now = c.now.Add(d)
	for _, timer := range c.timers {
		timer.fireIfDue()
	}
}

type fakeTimer struct {
	clock    *fakeClock
	deadline time.Time
	active   bool
	ch       chan time.Time
}

func (t *fakeTimer) C() <-chan time.Time {
	return t.ch
}

func (t *fakeTimer) Stop() bool {
	wasActive := t.active
	t.active = false
	return wasActive
}

func (t *fakeTimer) Reset(d time.Duration) bool {
	wasActive := t.active
	t.deadline = t.clock.Now().Add(d)
	t.active = true
	return wasActive
}

func (t *fakeTimer) fireIfDue() {
	if !t.active || t.clock.Now().Before(t.deadline) {
		return
	}
	t.active = false
	t.ch <- t.deadline
}
