// Package pretty prints NetworkEvents as one readable, optionally coloured line each.
package pretty

import (
	"fmt"
	"io"
	"sync"

	"github.com/minidd/topology/pkg/contract"
)

const (
	reset = "\x1b[0m"
	green = "\x1b[32m"
	red   = "\x1b[31;1m"
	grey  = "\x1b[90m"
)

type Printer struct {
	mu    sync.Mutex
	w     io.Writer
	color bool
}

func New(w io.Writer, color bool) *Printer { return &Printer{w: w, color: color} }

// Format renders one event, e.g.
//
//	09:30:01.412  CONNECT        orders(4121) → payments:8002
//	09:31:14.020  CONNECT_FAILED orders(4121) → payments:8002  ECONNREFUSED
func (p *Printer) Format(ev contract.NetworkEvent) string {
	src, dst := "?", "?"
	if ev.SrcService != nil {
		src = *ev.SrcService
	}
	if ev.DstService != nil {
		dst = *ev.DstService
	}
	line := fmt.Sprintf("%s  %-14s %s(%d) → %s:%d",
		ev.Timestamp.Time.Local().Format("15:04:05.000"), string(ev.EventType), src, ev.PID, dst, ev.DstPort)

	colour := ""
	switch ev.EventType {
	case contract.EventConnect, contract.EventAccept:
		colour = green
	case contract.EventClose:
		colour = grey
		if ev.DurationMS != nil {
			line += fmt.Sprintf("  lived %.1fms", *ev.DurationMS)
		}
	case contract.EventConnectFailed:
		colour = red
		if ev.Error != nil {
			line += "  " + *ev.Error
		}
	}
	if p.color && colour != "" {
		line = colour + line + reset
	}
	return line
}

// Print writes one line to the underlying writer.
func (p *Printer) Print(ev contract.NetworkEvent) {
	line := p.Format(ev)
	p.mu.Lock()
	defer p.mu.Unlock()
	fmt.Fprintln(p.w, line)
}
