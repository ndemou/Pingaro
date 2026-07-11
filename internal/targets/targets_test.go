package targets

import "testing"

func TestParsePreservesSpecialNames(t *testing.T) {
	got := Parse("localhost, gateway; 1.1.1.1")
	want := []string{"localhost", "gateway", "1.1.1.1"}
	if len(got) != len(want) {
		t.Fatalf("Parse length = %d, want %d: %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Parse[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestResolveReplacesSpecialNames(t *testing.T) {
	got := Resolve([]string{"localhost", "gateway", "8.8.8.8"}, "192.168.1.1")
	want := []string{"127.0.0.1", "192.168.1.1", "8.8.8.8"}
	if len(got) != len(want) {
		t.Fatalf("Resolve length = %d, want %d: %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Resolve[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestResolveDropsGatewayWhenGatewayMissing(t *testing.T) {
	got := Resolve([]string{"gateway", "localhost"}, "")
	want := []string{"127.0.0.1"}
	if len(got) != len(want) {
		t.Fatalf("Resolve length = %d, want %d: %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Resolve[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestNeedsGateway(t *testing.T) {
	if !NeedsGateway([]string{"localhost", " Gateway "}) {
		t.Fatal("NeedsGateway returned false for gateway target")
	}
	if NeedsGateway([]string{"localhost", "1.1.1.1"}) {
		t.Fatal("NeedsGateway returned true without gateway target")
	}
}
