//go:build windows && integration

package probe

import (
	"context"
	"testing"
	"time"
)

func TestICMPProberLocalhostIntegration(t *testing.T) {
	req := Request{
		ID:       1,
		Target:   "127.0.0.1",
		SentAt:   time.Now(),
		Deadline: time.Now().Add(500 * time.Millisecond),
	}
	got := NewICMPProber().Probe(context.Background(), req)
	if got.Kind() != OutcomeReply {
		t.Fatalf("localhost ICMP outcome = %v detail %q", got.Kind(), got.Detail())
	}
}
