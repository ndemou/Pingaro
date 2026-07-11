package profiles

import "testing"

func TestDefaultUseTypes(t *testing.T) {
	got := set(DefaultUseTypes())
	for _, name := range []string{"Browsing & Email", "Audio Calls", "Video Calls", "Online Gaming"} {
		if !got[name] {
			t.Fatalf("default use types missing %q: %v", name, DefaultUseTypes())
		}
	}
	for _, name := range []string{"Remote Desktop", "Superhuman Gaming"} {
		if got[name] {
			t.Fatalf("default use types unexpectedly include %q: %v", name, DefaultUseTypes())
		}
	}
}

func TestThresholds(t *testing.T) {
	tests := []struct {
		name   string
		rtt    [3]float64
		loss   [3]float64
		jitter [3]float64
	}{
		{name: "Browsing & Email", rtt: [3]float64{150, 300, 600}, loss: [3]float64{0.5, 2, 5}, jitter: [3]float64{600, 600, 600}},
		{name: "Remote Desktop", rtt: [3]float64{100, 150, 220}, loss: [3]float64{1, 2, 3}, jitter: [3]float64{15, 30, 50}},
		{name: "Audio Calls", rtt: [3]float64{100, 150, 250}, loss: [3]float64{1, 2, 3}, jitter: [3]float64{20, 30, 50}},
		{name: "Video Calls", rtt: [3]float64{100, 150, 250}, loss: [3]float64{1, 2, 3}, jitter: [3]float64{20, 30, 50}},
		{name: "Online Gaming", rtt: [3]float64{50, 80, 120}, loss: [3]float64{0.5, 1, 2}, jitter: [3]float64{10, 20, 30}},
		{name: "Superhuman Gaming", rtt: [3]float64{20, 35, 60}, loss: [3]float64{0.1, 0.5, 1}, jitter: [3]float64{5, 10, 20}},
	}
	for _, tt := range tests {
		got, ok := ForName(tt.name)
		if !ok {
			t.Fatalf("ForName(%q) returned false", tt.name)
		}
		if got.RTT != tt.rtt {
			t.Fatalf("%s RTT thresholds = %v, want %v", tt.name, got.RTT, tt.rtt)
		}
		if got.Loss != tt.loss {
			t.Fatalf("%s loss thresholds = %v, want %v", tt.name, got.Loss, tt.loss)
		}
		if got.Jitter != tt.jitter {
			t.Fatalf("%s jitter thresholds = %v, want %v", tt.name, got.Jitter, tt.jitter)
		}
	}
}

func TestForUsesUsesMostDemandingThresholds(t *testing.T) {
	got := ForUses([]string{"Browsing & Email", "Online Gaming", "Video Calls"})
	if got.RTT != [3]float64{50, 80, 120} {
		t.Fatalf("RTT thresholds = %v, want [50 80 120]", got.RTT)
	}
	if got.Loss != [3]float64{0.5, 1, 2} {
		t.Fatalf("loss thresholds = %v, want [0.5 1 2]", got.Loss)
	}
	if got.Jitter != [3]float64{10, 20, 30} {
		t.Fatalf("jitter thresholds = %v, want [10 20 30]", got.Jitter)
	}
}

func TestNormalizeUseTypesRenamesLegacyValues(t *testing.T) {
	got := NormalizeUseTypes([]string{"low latency gaming", "email & browsing"}, "")
	if len(got) != 2 || got[0] != "Superhuman Gaming" || got[1] != "Browsing & Email" {
		t.Fatalf("NormalizeUseTypes() = %v, want legacy names normalized", got)
	}
}

func TestUsesShowJitterOnlyForAudioOrVideo(t *testing.T) {
	if UsesShowJitter([]string{"Browsing & Email", "Online Gaming"}) {
		t.Fatal("UsesShowJitter returned true without audio or video calls")
	}
	if !UsesShowJitter([]string{"Audio Calls"}) {
		t.Fatal("UsesShowJitter returned false for Audio Calls")
	}
	if !UsesShowJitter([]string{"Video Calls"}) {
		t.Fatal("UsesShowJitter returned false for Video Calls")
	}
}

func TestThresholdSeverity(t *testing.T) {
	thresholds := [3]float64{10, 20, 30}
	tests := []struct {
		value float64
		want  Severity
	}{
		{value: 9, want: SeverityGood},
		{value: 10, want: SeverityMinor},
		{value: 20, want: SeverityNoticeable},
		{value: 30, want: SeveritySerious},
	}
	for _, tt := range tests {
		if got := ThresholdSeverity(tt.value, thresholds); got != tt.want {
			t.Fatalf("ThresholdSeverity(%v) = %v, want %v", tt.value, got, tt.want)
		}
	}
}

func set(values []string) map[string]bool {
	out := make(map[string]bool, len(values))
	for _, value := range values {
		out[value] = true
	}
	return out
}
