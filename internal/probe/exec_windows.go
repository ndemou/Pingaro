package probe

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/netip"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"
)

var (
	reTime   = regexp.MustCompile(`time[=<]([0-9]+)ms`)
	reTarget = regexp.MustCompile(`Reply from ([^:]+):`)
)

type ExecProber struct{}

func NewExecProber() ExecProber {
	return ExecProber{}
}

func (ExecProber) Probe(ctx context.Context, req Request) Outcome {
	cctx, cancel := context.WithTimeout(ctx, 2500*time.Millisecond)
	defer cancel()

	cmd := exec.CommandContext(cctx, "ping", "-n", "1", "-w", "1000", req.Target)
	cmd.SysProcAttr = hiddenSysProcAttr()
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	return parsePingOutput(req, buf.String(), err)
}

func parsePingOutput(req Request, text string, err error) Outcome {
	if m := reTime.FindStringSubmatch(text); len(m) == 2 {
		rtt, _ := strconv.Atoi(m[1])
		if rtt < 1 {
			rtt = 1
		}
		var address netip.Addr
		if dm := reTarget.FindStringSubmatch(text); len(dm) == 2 {
			address, _ = netip.ParseAddr(strings.TrimSpace(dm[1]))
		}
		return NewReply(req, address, time.Duration(rtt)*time.Millisecond)
	}

	detail := firstLine(text)
	lowerText := strings.ToLower(text)
	switch {
	case errors.Is(err, context.Canceled):
		return NewCancelled(req).WithDetail(detail)
	case strings.Contains(lowerText, "ttl expired"):
		return NewTTLExpired(req, 0, detail)
	case strings.Contains(lowerText, "unreachable"):
		return NewUnreachable(req, 0, detail)
	case strings.Contains(lowerText, "timed out"):
		return NewTimeout(req).WithDetail(detail)
	case err != nil && !errors.Is(err, context.DeadlineExceeded):
		if detail != "" {
			return NewLocalFailure(req, fmt.Errorf("%w: %s", err, detail))
		}
		return NewLocalFailure(req, err)
	default:
		return NewTimeout(req).WithDetail(detail)
	}
}

func hiddenSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: 0x08000000, // CREATE_NO_WINDOW
	}
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if i := strings.IndexAny(s, "\r\n"); i >= 0 {
		return s[:i]
	}
	return s
}
