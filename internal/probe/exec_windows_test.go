package probe

import (
	"errors"
	"net/netip"
	"testing"
	"time"
)

func TestParsePingOutputPreservesRequestAndReplyFields(t *testing.T) {
	req := Request{ID: 42, SessionID: 7, GroupID: 2, Target: "example.test", SentAt: time.Date(2026, time.July, 11, 10, 0, 0, 0, time.UTC)}
	text := "Reply from 192.0.2.10: bytes=32 time=12ms TTL=57\r\n"

	got := parsePingOutput(req, text, nil)
	if got.Request() != req {
		t.Fatalf("request = %+v, want %+v", got.Request(), req)
	}
	rtt, ok := got.RTT()
	if got.Kind() != OutcomeReply || !ok || rtt != 12*time.Millisecond {
		t.Fatalf("reply outcome = kind %v rtt %v ok %v", got.Kind(), rtt, ok)
	}
	address, ok := got.Address()
	if !ok || address != netip.MustParseAddr("192.0.2.10") {
		t.Fatalf("address = %v ok %v, want 192.0.2.10", address, ok)
	}
	if got.Detail() != "" {
		t.Fatalf("detail = %q, want empty", got.Detail())
	}
}

func TestParsePingOutputDistinguishesTimeoutFromLocalFailure(t *testing.T) {
	req := Request{ID: 1, Target: "192.0.2.1"}

	timeout := parsePingOutput(req, "Request timed out.\r\n", nil)
	if timeout.Kind() != OutcomeTimeout || !timeout.CountsAsNetworkLoss() {
		t.Fatalf("timeout outcome = kind %v networkLoss %v", timeout.Kind(), timeout.CountsAsNetworkLoss())
	}

	failure := parsePingOutput(req, "Ping request could not find host missing.\r\n", errors.New("exit status 1"))
	if failure.Kind() != OutcomeLocalFailure || failure.CountsAsNetworkLoss() {
		t.Fatalf("failure outcome = kind %v networkLoss %v", failure.Kind(), failure.CountsAsNetworkLoss())
	}
}
