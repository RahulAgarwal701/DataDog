// Package convert turns raw kernel events into contract NetworkEvents.
package convert

import (
	"bytes"
	"math"
	"net"
	"time"

	"github.com/minidd/ebpf-agent/internal/raw"
	"github.com/minidd/ebpf-agent/internal/resolve"
	"github.com/minidd/topology/pkg/contract"
)

// Drop says why an event was not emitted (Keep = emitted).
type Drop int

const (
	Keep Drop = iota
	DropUnregisteredPort
	DropUnresolvedSource
	DropUnknownKind
	DropLoopback
)

type Converter struct {
	Host           string
	Res            *resolve.Resolver
	MonoBase       time.Time // wall-clock time corresponding to CLOCK_MONOTONIC == 0
	EmitUnresolved bool      // keep events even if source/destination cannot be resolved (debugging)
}

// Convert builds the contract event. Unless EmitUnresolved is set, an event is only
// kept when BOTH endpoints resolve to a service name, so unrelated host traffic
// (Prometheus scrapes, Elasticsearch, DNS, ...) never pollutes the topology.
func (c *Converter) Convert(r raw.Event) (contract.NetworkEvent, Drop) {
	srcIP := net.IP(r.SAddr[:])
	dstIP := net.IP(r.DAddr[:])

	// A service's healthcheck dials its own port over loopback, and the probe
	// process inherits the container's SERVICE_NAME. Those show up as a node
	// calling itself ("orders -> orders") every few seconds, so drop loopback:
	// the topology is about how services talk to *each other*.
	if !c.EmitUnresolved && srcIP.IsLoopback() && dstIP.IsLoopback() {
		return contract.NetworkEvent{}, DropLoopback
	}

	dstName, dstOK := c.Res.Dst(int(r.DPort))
	srcName, srcOK := c.Res.Src(r.PID)
	if !c.EmitUnresolved {
		if !dstOK {
			return contract.NetworkEvent{}, DropUnregisteredPort
		}
		if !srcOK {
			return contract.NetworkEvent{}, DropUnresolvedSource
		}
	}

	ev := contract.NetworkEvent{
		EventID:     contract.NewUUID(),
		Timestamp:   contract.TS(c.MonoBase.Add(time.Duration(r.TsNs))),
		Host:        c.Host,
		Protocol:    "tcp",
		SrcIP:       srcIP.String(),
		SrcPort:     int(r.SPort),
		DstIP:       dstIP.String(),
		DstPort:     int(r.DPort),
		PID:         int(r.PID),
		ProcessName: commString(r.Comm),
	}
	if srcOK {
		ev.SrcService = contract.Ptr(srcName)
	}
	if dstOK {
		ev.DstService = contract.Ptr(dstName)
	}

	switch r.Kind {
	case raw.KindConnect:
		ev.EventType = contract.EventConnect
	case raw.KindConnectFailed:
		ev.EventType = contract.EventConnectFailed
		ev.Error = contract.Ptr(failureError(time.Duration(r.DurationNs)))
	case raw.KindClose:
		ev.EventType = contract.EventClose
		ev.DurationMS = contract.Ptr(math.Round(float64(r.DurationNs)/1e3) / 1e3) // ms, 3 decimals
	default:
		return contract.NetworkEvent{}, DropUnknownKind
	}
	return ev, Keep
}

// failureError classifies a failed connect() from how long it took to fail.
// A refused connection gets an RST almost immediately; a timeout takes seconds
// (SYN retransmits). The tracepoint does not expose sk_err, so this is a heuristic.
func failureError(d time.Duration) string {
	if d < time.Second {
		return "ECONNREFUSED"
	}
	return "ETIMEDOUT"
}

func commString(c [16]byte) string {
	if i := bytes.IndexByte(c[:], 0); i >= 0 {
		return string(c[:i])
	}
	return string(c[:])
}
