package probe

import (
	"net/netip"
	"testing"
	"time"
)

func TestOutcomeInvariants(t *testing.T) {
	req := Request{ID: 99, Target: "example.test"}
	reply := NewReply(req, netip.MustParseAddr("192.0.2.10"), 42*time.Millisecond)
	if reply.Kind() != OutcomeReply {
		t.Fatalf("reply kind = %v", reply.Kind())
	}
	if rtt, ok := reply.RTT(); !ok || rtt != 42*time.Millisecond {
		t.Fatalf("reply RTT = %v ok %v, want 42ms true", rtt, ok)
	}

	for _, outcome := range []Outcome{
		NewTimeout(req),
		NewUnreachable(req, 0, "unreachable"),
		NewTTLExpired(req, 0, "ttl expired"),
		NewCancelled(req),
		NewLocalFailure(req, errTest),
		NewNotSent(req, "capacity"),
	} {
		if rtt, ok := outcome.RTT(); ok || rtt != 0 {
			t.Fatalf("%v RTT = %v ok %v, want no RTT", outcome.Kind(), rtt, ok)
		}
	}
}

func TestOutcomeLossClassification(t *testing.T) {
	req := Request{ID: 1, Target: "example.test"}
	tests := []struct {
		outcome Outcome
		want    bool
	}{
		{NewReply(req, netip.MustParseAddr("192.0.2.10"), time.Millisecond), false},
		{NewTimeout(req), true},
		{NewUnreachable(req, 0, ""), true},
		{NewTTLExpired(req, 0, ""), true},
		{NewCancelled(req), false},
		{NewLocalFailure(req, errTest), false},
		{NewNotSent(req, "capacity"), false},
	}
	for _, tt := range tests {
		if got := tt.outcome.CountsAsNetworkLoss(); got != tt.want {
			t.Fatalf("%v CountsAsNetworkLoss() = %v, want %v", tt.outcome.Kind(), got, tt.want)
		}
	}
}

func TestNewReplyRejectsNegativeRTT(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("NewReply with negative RTT did not panic")
		}
	}()
	NewReply(Request{}, netip.Addr{}, -time.Millisecond)
}

type testError string

func (e testError) Error() string { return string(e) }

const errTest = testError("local")
