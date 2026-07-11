package probe

import (
	"bytes"
	"context"
	"errors"
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
		dest := req.Target
		if dm := reTarget.FindStringSubmatch(text); len(dm) == 2 {
			dest = strings.TrimSpace(dm[1])
		}
		return NewOutcome(req, rtt, dest, "Success", "")
	}

	status := "TimeOut"
	if err != nil && !errors.Is(err, context.DeadlineExceeded) {
		status = "PingFailed"
	}
	return NewOutcome(req, 0, req.Target, status, firstLine(text))
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
