//go:build windows

package targets

import (
	"math"
	"net"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
)

func DefaultGateway() string {
	cmd := exec.Command("route", "PRINT", "-4", "0.0.0.0")
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: 0x08000000, // CREATE_NO_WINDOW
	}
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return defaultGatewayFromRoutePrint(string(out))
}

func defaultGatewayFromRoutePrint(output string) string {
	bestGateway := ""
	bestMetric := math.MaxInt
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 5 || fields[0] != "0.0.0.0" || fields[1] != "0.0.0.0" {
			continue
		}
		gateway := fields[2]
		if net.ParseIP(gateway) == nil {
			continue
		}
		metric := parseMetric(fields[len(fields)-1])
		if metric < bestMetric {
			bestMetric = metric
			bestGateway = gateway
		}
	}
	return bestGateway
}

func parseMetric(value string) int {
	n, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return math.MaxInt
	}
	return n
}
