package probe

import (
	"net/netip"
	"testing"
	"time"
)

func TestICMPStatusTranslation(t *testing.T) {
	req := Request{ID: 1, Target: "192.0.2.1"}
	tests := []struct {
		status uint32
		want   OutcomeKind
		loss   bool
	}{
		{status: ipReqTimedOut, want: OutcomeTimeout, loss: true},
		{status: ipDestHostUnreachable, want: OutcomeUnreachable, loss: true},
		{status: ipTTLExpiredTransit, want: OutcomeTTLExpired, loss: true},
		{status: 123456, want: OutcomeLocalFailure, loss: false},
	}
	for _, tt := range tests {
		got := outcomeFromICMPStatus(req, tt.status, "detail")
		if got.Kind() != tt.want || got.CountsAsNetworkLoss() != tt.loss {
			t.Fatalf("status %d -> kind %v loss %v, want %v/%v", tt.status, got.Kind(), got.CountsAsNetworkLoss(), tt.want, tt.loss)
		}
		if got.Request() != req {
			t.Fatalf("request = %+v, want %+v", got.Request(), req)
		}
	}
}

func TestICMPAddressConversionRoundTripsIPv4(t *testing.T) {
	addr := netip.MustParseAddr("127.0.0.1")
	if got := ipAddrToNetip(netipToIPAddr(addr)); got != addr {
		t.Fatalf("round-trip address = %v, want %v", got, addr)
	}
}

func TestICMPTimeoutMillisecondsRoundsUp(t *testing.T) {
	tests := []struct {
		in   time.Duration
		want uint32
	}{
		{in: time.Nanosecond, want: 1},
		{in: time.Millisecond, want: 1},
		{in: 1500 * time.Microsecond, want: 2},
	}
	for _, tt := range tests {
		if got := timeoutMilliseconds(tt.in); got != tt.want {
			t.Fatalf("timeoutMilliseconds(%v) = %d, want %d", tt.in, got, tt.want)
		}
	}
}
