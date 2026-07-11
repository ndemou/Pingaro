//go:build windows

package targets

import "testing"

func TestDefaultGatewayFromRoutePrintChoosesLowestMetric(t *testing.T) {
	output := `
IPv4 Route Table
===========================================================================
Active Routes:
Network Destination        Netmask          Gateway       Interface  Metric
          0.0.0.0          0.0.0.0     192.168.1.1   192.168.1.10     50
          0.0.0.0          0.0.0.0     10.0.0.1      10.0.0.10        20
`
	got := defaultGatewayFromRoutePrint(output)
	if got != "10.0.0.1" {
		t.Fatalf("defaultGatewayFromRoutePrint() = %q, want 10.0.0.1", got)
	}
}
