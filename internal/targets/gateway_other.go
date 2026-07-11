//go:build !windows

package targets

import (
	"net"
	"os/exec"
	"strings"
)

func DefaultGateway() string {
	cmd := exec.Command("sh", "-c", "ip route show default 2>/dev/null | head -n 1")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	fields := strings.Fields(string(out))
	for i := 0; i+1 < len(fields); i++ {
		if fields[i] == "via" && net.ParseIP(fields[i+1]) != nil {
			return fields[i+1]
		}
	}
	return ""
}
