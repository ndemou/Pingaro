package probe

import (
	"errors"
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
	if got.Status() != "Success" || got.RTTMilliseconds() != 12 || got.Destination() != "192.0.2.10" {
		t.Fatalf("reply outcome = status %q rtt %d destination %q", got.Status(), got.RTTMilliseconds(), got.Destination())
	}
	if got.Warning() != "" {
		t.Fatalf("warning = %q, want empty", got.Warning())
	}
}

func TestParsePingOutputDistinguishesTimeoutFromLocalFailure(t *testing.T) {
	req := Request{ID: 1, Target: "192.0.2.1"}

	timeout := parsePingOutput(req, "Request timed out.\r\n", nil)
	if timeout.Status() != "TimeOut" || timeout.RTTMilliseconds() != 0 || timeout.Warning() != "Request timed out." {
		t.Fatalf("timeout outcome = status %q rtt %d warning %q", timeout.Status(), timeout.RTTMilliseconds(), timeout.Warning())
	}

	failure := parsePingOutput(req, "Ping request could not find host missing.\r\n", errors.New("exit status 1"))
	if failure.Status() != "PingFailed" || failure.RTTMilliseconds() != 0 {
		t.Fatalf("failure outcome = status %q rtt %d", failure.Status(), failure.RTTMilliseconds())
	}
}
