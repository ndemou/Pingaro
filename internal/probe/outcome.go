package probe

import (
	"fmt"
	"net/netip"
	"time"
)

type OutcomeKind uint8

const (
	OutcomeReply OutcomeKind = iota
	OutcomeTimeout
	OutcomeUnreachable
	OutcomeTTLExpired
	OutcomeCancelled
	OutcomeLocalFailure
	OutcomeNotSent
)

func (kind OutcomeKind) String() string {
	switch kind {
	case OutcomeReply:
		return "reply"
	case OutcomeTimeout:
		return "timeout"
	case OutcomeUnreachable:
		return "unreachable"
	case OutcomeTTLExpired:
		return "ttl_expired"
	case OutcomeCancelled:
		return "cancelled"
	case OutcomeLocalFailure:
		return "local_failure"
	case OutcomeNotSent:
		return "not_sent"
	default:
		return "unknown"
	}
}

func (kind OutcomeKind) CountsAsNetworkLoss() bool {
	switch kind {
	case OutcomeTimeout, OutcomeUnreachable, OutcomeTTLExpired:
		return true
	default:
		return false
	}
}

type Outcome struct {
	request Request
	kind    OutcomeKind
	address netip.Addr
	rtt     time.Duration
	code    uint32
	detail  string
	err     error
}

func NewReply(req Request, address netip.Addr, rtt time.Duration) Outcome {
	if rtt < 0 {
		panic("probe reply RTT cannot be negative")
	}
	return Outcome{request: req, kind: OutcomeReply, address: address, rtt: rtt}
}

func NewTimeout(req Request) Outcome {
	return Outcome{request: req, kind: OutcomeTimeout}
}

func NewUnreachable(req Request, code uint32, detail string) Outcome {
	return Outcome{request: req, kind: OutcomeUnreachable, code: code, detail: detail}
}

func NewTTLExpired(req Request, code uint32, detail string) Outcome {
	return Outcome{request: req, kind: OutcomeTTLExpired, code: code, detail: detail}
}

func NewCancelled(req Request) Outcome {
	return Outcome{request: req, kind: OutcomeCancelled}
}

func NewLocalFailure(req Request, err error) Outcome {
	return Outcome{request: req, kind: OutcomeLocalFailure, err: err}
}

func NewNotSent(req Request, reason string) Outcome {
	return Outcome{request: req, kind: OutcomeNotSent, detail: reason}
}

func (o Outcome) Request() Request {
	return o.request
}

func (o Outcome) Kind() OutcomeKind {
	return o.kind
}

func (o Outcome) RTT() (time.Duration, bool) {
	if o.kind != OutcomeReply {
		return 0, false
	}
	return o.rtt, true
}

func (o Outcome) Address() (netip.Addr, bool) {
	if o.kind != OutcomeReply || !o.address.IsValid() {
		return netip.Addr{}, false
	}
	return o.address, true
}

func (o Outcome) Code() uint32 {
	return o.code
}

func (o Outcome) Detail() string {
	if o.detail != "" {
		return o.detail
	}
	if o.err != nil {
		return o.err.Error()
	}
	return ""
}

func (o Outcome) Err() error {
	return o.err
}

func (o Outcome) WithDetail(detail string) Outcome {
	o.detail = detail
	return o
}

func (o Outcome) CountsAsNetworkLoss() bool {
	return o.kind.CountsAsNetworkLoss()
}

func (o Outcome) String() string {
	if detail := o.Detail(); detail != "" {
		return fmt.Sprintf("%s: %s", o.kind, detail)
	}
	return o.kind.String()
}
