package probe

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"net"
	"net/netip"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	ipSuccess               = 0
	ipDestNetUnreachable    = 11002
	ipDestHostUnreachable   = 11003
	ipDestProtoUnreachable  = 11004
	ipDestPortUnreachable   = 11005
	ipReqTimedOut           = 11010
	ipTTLExpiredTransit     = 11013
	ipTTLExpiredReassembly  = 11014
	defaultICMPReplyTimeout = 500 * time.Millisecond
)

var (
	iphlpapi          = windows.NewLazySystemDLL("iphlpapi.dll")
	procIcmpCreate    = iphlpapi.NewProc("IcmpCreateFile")
	procIcmpClose     = iphlpapi.NewProc("IcmpCloseHandle")
	procIcmpSendEcho  = iphlpapi.NewProc("IcmpSendEcho")
	defaultICMPData   = []byte("pingaro")
	errNoIPv4ForProbe = fmt.Errorf("no IPv4 address found for target")
)

type Resolver interface {
	Lookup(context.Context, string) ([]netip.Addr, error)
}

type NetResolver struct{}

func (NetResolver) Lookup(ctx context.Context, target string) ([]netip.Addr, error) {
	return net.DefaultResolver.LookupNetIP(ctx, "ip4", target)
}

type ICMPProber struct {
	resolver Resolver
	now      func() time.Time
}

func NewICMPProber() *ICMPProber {
	return &ICMPProber{resolver: NetResolver{}, now: time.Now}
}

func NewICMPProberWithResolver(resolver Resolver) *ICMPProber {
	if resolver == nil {
		resolver = NetResolver{}
	}
	return &ICMPProber{resolver: resolver, now: time.Now}
}

func (p *ICMPProber) Probe(ctx context.Context, req Request) Outcome {
	if err := ctx.Err(); err != nil {
		return NewCancelled(req).WithDetail(err.Error())
	}
	if p.resolver == nil {
		p.resolver = NetResolver{}
	}
	if p.now == nil {
		p.now = time.Now
	}
	remaining := remainingDeadline(p.now(), req)
	if remaining <= 0 {
		return NewTimeout(req)
	}
	address, err := p.resolveIPv4(ctx, req.Target)
	if err != nil {
		if ctx.Err() != nil {
			return NewCancelled(req).WithDetail(ctx.Err().Error())
		}
		return NewLocalFailure(req, err)
	}
	remaining = remainingDeadline(p.now(), req)
	if remaining <= 0 {
		return NewTimeout(req)
	}
	return p.sendEcho(ctx, req, address, remaining)
}

func (p *ICMPProber) resolveIPv4(ctx context.Context, target string) (netip.Addr, error) {
	if addr, err := netip.ParseAddr(target); err == nil {
		if addr.Is4() {
			return addr, nil
		}
		return netip.Addr{}, errNoIPv4ForProbe
	}
	addresses, err := p.resolver.Lookup(ctx, target)
	if err != nil {
		return netip.Addr{}, fmt.Errorf("resolve %q: %w", target, err)
	}
	for _, addr := range addresses {
		if addr.Is4() {
			return addr, nil
		}
	}
	return netip.Addr{}, errNoIPv4ForProbe
}

func (p *ICMPProber) sendEcho(ctx context.Context, req Request, address netip.Addr, timeout time.Duration) Outcome {
	handle, err := icmpCreateFile()
	if err != nil {
		return NewLocalFailure(req, err)
	}
	defer icmpCloseHandle(handle)

	replySize := int(unsafe.Sizeof(icmpEchoReply{})) + len(defaultICMPData) + 8
	replyBuffer := make([]byte, replySize)
	timeoutMS := timeoutMilliseconds(timeout)
	count, callErr := icmpSendEcho(handle, address, defaultICMPData, replyBuffer, timeoutMS)
	if err := ctx.Err(); err != nil {
		return NewCancelled(req).WithDetail(err.Error())
	}
	if count == 0 {
		status := errnoStatus(callErr)
		if status == 0 {
			status = ipReqTimedOut
		}
		return outcomeFromICMPStatus(req, status, callErr.Error())
	}
	reply := (*icmpEchoReply)(unsafe.Pointer(&replyBuffer[0]))
	if reply.Status != ipSuccess {
		return outcomeFromICMPStatus(req, reply.Status, "")
	}
	rtt := time.Duration(reply.RoundTripTime) * time.Millisecond
	if !req.Deadline.IsZero() && p.now().After(req.Deadline) {
		return NewTimeout(req)
	}
	if allowed := req.Deadline.Sub(req.SentAt); !req.SentAt.IsZero() && !req.Deadline.IsZero() && rtt > allowed {
		return NewTimeout(req)
	}
	return NewReply(req, ipAddrToNetip(reply.Address), rtt)
}

func remainingDeadline(now time.Time, req Request) time.Duration {
	if req.Deadline.IsZero() {
		return defaultICMPReplyTimeout
	}
	return req.Deadline.Sub(now)
}

func timeoutMilliseconds(timeout time.Duration) uint32 {
	if timeout <= 0 {
		return 0
	}
	ms := int64(math.Ceil(float64(timeout) / float64(time.Millisecond)))
	if ms < 1 {
		return 1
	}
	if ms > math.MaxUint32 {
		return math.MaxUint32
	}
	return uint32(ms)
}

type ipOptionInformation struct {
	TTL         byte
	TOS         byte
	Flags       byte
	OptionsSize byte
	OptionsData uintptr
}

type icmpEchoReply struct {
	Address       uint32
	Status        uint32
	RoundTripTime uint32
	DataSize      uint16
	Reserved      uint16
	Data          uintptr
	Options       ipOptionInformation
}

func icmpCreateFile() (windows.Handle, error) {
	handle, _, err := procIcmpCreate.Call()
	if handle == 0 || windows.Handle(handle) == windows.InvalidHandle {
		return 0, fmt.Errorf("IcmpCreateFile: %w", err)
	}
	return windows.Handle(handle), nil
}

func icmpCloseHandle(handle windows.Handle) {
	_, _, _ = procIcmpClose.Call(uintptr(handle))
}

func icmpSendEcho(handle windows.Handle, address netip.Addr, data []byte, reply []byte, timeoutMS uint32) (uint32, error) {
	var dataPtr uintptr
	if len(data) > 0 {
		dataPtr = uintptr(unsafe.Pointer(&data[0]))
	}
	count, _, err := procIcmpSendEcho.Call(
		uintptr(handle),
		uintptr(netipToIPAddr(address)),
		dataPtr,
		uintptr(uint16(len(data))),
		0,
		uintptr(unsafe.Pointer(&reply[0])),
		uintptr(uint32(len(reply))),
		uintptr(timeoutMS),
	)
	return uint32(count), err
}

func netipToIPAddr(address netip.Addr) uint32 {
	as4 := address.As4()
	return binary.LittleEndian.Uint32(as4[:])
}

func ipAddrToNetip(address uint32) netip.Addr {
	var as4 [4]byte
	binary.LittleEndian.PutUint32(as4[:], address)
	return netip.AddrFrom4(as4)
}

func errnoStatus(err error) uint32 {
	if errno, ok := err.(windows.Errno); ok {
		return uint32(errno)
	}
	return 0
}

func outcomeFromICMPStatus(req Request, status uint32, detail string) Outcome {
	switch status {
	case ipSuccess:
		return NewReply(req, netip.Addr{}, 0)
	case ipReqTimedOut:
		return NewTimeout(req).WithDetail(detail)
	case ipDestNetUnreachable, ipDestHostUnreachable, ipDestProtoUnreachable, ipDestPortUnreachable:
		return NewUnreachable(req, status, detail)
	case ipTTLExpiredTransit, ipTTLExpiredReassembly:
		return NewTTLExpired(req, status, detail)
	default:
		if detail == "" {
			detail = fmt.Sprintf("ICMP status %d", status)
		}
		return NewLocalFailure(req, errors.New(detail))
	}
}
