//go:build linux

// Package loader loads the compiled eBPF program, attaches it to the
// sock:inet_sock_set_state tracepoint and streams raw events from the ring buffer.
package loader

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/cilium/ebpf/link"
	"github.com/cilium/ebpf/ringbuf"
	"github.com/cilium/ebpf/rlimit"
	"golang.org/x/sys/unix"

	"github.com/minidd/ebpf-agent/internal/raw"
)

type Loader struct {
	objs netmonObjects
	tp   link.Link
	rd   *ringbuf.Reader

	closeOnce sync.Once
	closeErr  error
}

// Load loads and attaches the program. Requires root (or CAP_BPF + CAP_PERFMON).
func Load() (*Loader, error) {
	if err := rlimit.RemoveMemlock(); err != nil {
		return nil, fmt.Errorf("remove memlock rlimit: %w", err)
	}
	l := &Loader{}
	if err := loadNetmonObjects(&l.objs, nil); err != nil {
		return nil, fmt.Errorf("load eBPF objects: %w", err)
	}
	tp, err := link.Tracepoint("sock", "inet_sock_set_state", l.objs.HandleSetState, nil)
	if err != nil {
		l.objs.Close()
		return nil, fmt.Errorf("attach tracepoint sock:inet_sock_set_state: %w", err)
	}
	l.tp = tp
	rd, err := ringbuf.NewReader(l.objs.Events)
	if err != nil {
		l.tp.Close()
		l.objs.Close()
		return nil, fmt.Errorf("open ring buffer: %w", err)
	}
	l.rd = rd
	return l, nil
}

// Read blocks until the next event. After Close it returns an error for which IsClosed is true.
func (l *Loader) Read() (raw.Event, error) {
	rec, err := l.rd.Read()
	if err != nil {
		return raw.Event{}, err
	}
	return raw.Decode(rec.RawSample)
}

// IsClosed reports whether err means the loader was closed.
func IsClosed(err error) bool { return errors.Is(err, ringbuf.ErrClosed) }

// Close detaches the program and frees all kernel resources. It is safe to call more than once.
func (l *Loader) Close() error {
	l.closeOnce.Do(func() { l.closeErr = l.close() })
	return l.closeErr
}

func (l *Loader) close() error {
	var first error
	if l.rd != nil {
		if err := l.rd.Close(); err != nil {
			first = err
		}
	}
	if l.tp != nil {
		if err := l.tp.Close(); err != nil && first == nil {
			first = err
		}
	}
	if err := l.objs.Close(); err != nil && first == nil {
		first = err
	}
	return first
}

// MonoBase returns the wall-clock time at which CLOCK_MONOTONIC was 0. Adding an event's
// ts_ns to it gives the event's wall-clock time.
func MonoBase() (time.Time, error) {
	var ts unix.Timespec
	if err := unix.ClockGettime(unix.CLOCK_MONOTONIC, &ts); err != nil {
		return time.Time{}, fmt.Errorf("clock_gettime: %w", err)
	}
	return time.Now().Add(-time.Duration(ts.Nano())), nil
}
