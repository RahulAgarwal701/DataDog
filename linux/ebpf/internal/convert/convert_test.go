package convert

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/minidd/ebpf-agent/internal/raw"
	"github.com/minidd/ebpf-agent/internal/resolve"
	"github.com/minidd/topology/pkg/contract"
	"github.com/minidd/topology/pkg/registry"
)

const regYAML = `
services:
  - {name: api-gateway, port: 8000, kind: service}
  - {name: orders,      port: 8001, kind: service}
  - {name: payments,    port: 8002, kind: service}
`

func comm(s string) (c [16]byte) {
	copy(c[:], s)
	return
}

func newConverter(t *testing.T) *Converter {
	t.Helper()
	reg, err := registry.Parse([]byte(regYAML))
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	dir := filepath.Join(root, "4121")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "environ"), []byte("SERVICE_NAME=orders\x00"), 0o644); err != nil {
		t.Fatal(err)
	}
	return &Converter{
		Host:     "demo-vm",
		Res:      resolve.New(reg, root),
		MonoBase: time.Date(2026, 10, 14, 9, 30, 0, 0, time.UTC),
	}
}

func baseRaw(kind uint32) raw.Event {
	return raw.Event{
		TsNs: 5_000_000_000, PID: 4121, Kind: kind,
		SAddr: [4]byte{172, 18, 0, 4}, DAddr: [4]byte{172, 18, 0, 5},
		SPort: 51522, DPort: 8002, Comm: comm("python"),
	}
}

func TestConnect(t *testing.T) {
	c := newConverter(t)
	ev, drop := c.Convert(baseRaw(raw.KindConnect))
	if drop != Keep {
		t.Fatalf("dropped: %v", drop)
	}
	if ev.EventType != contract.EventConnect || ev.SrcService == nil || *ev.SrcService != "orders" ||
		ev.DstService == nil || *ev.DstService != "payments" {
		t.Fatalf("wrong event: %+v", ev)
	}
	if ev.SrcIP != "172.18.0.4" || ev.DstIP != "172.18.0.5" || ev.DstPort != 8002 || ev.SrcPort != 51522 {
		t.Fatalf("wrong addressing: %+v", ev)
	}
	if ev.ProcessName != "python" || ev.PID != 4121 || ev.Host != "demo-vm" {
		t.Fatalf("wrong process info: %+v", ev)
	}
	if got := ev.Timestamp.Time.Format(time.RFC3339Nano); got != "2026-10-14T09:30:05Z" {
		t.Fatalf("timestamp = %s", got)
	}
	if ev.DurationMS != nil || ev.Error != nil {
		t.Fatalf("CONNECT must not carry duration_ms/error: %+v", ev)
	}
	if err := ev.Validate(); err != nil {
		t.Fatalf("event violates contract: %v", err)
	}
}

func TestConnectFailedClassification(t *testing.T) {
	c := newConverter(t)
	r := baseRaw(raw.KindConnectFailed)
	r.DurationNs = uint64(300 * time.Microsecond)
	ev, _ := c.Convert(r)
	if ev.EventType != contract.EventConnectFailed || ev.Error == nil || *ev.Error != "ECONNREFUSED" {
		t.Fatalf("fast failure should be ECONNREFUSED: %+v", ev)
	}
	r.DurationNs = uint64(3 * time.Second)
	ev, _ = c.Convert(r)
	if ev.Error == nil || *ev.Error != "ETIMEDOUT" {
		t.Fatalf("slow failure should be ETIMEDOUT: %+v", ev)
	}
}

func TestCloseDuration(t *testing.T) {
	c := newConverter(t)
	r := baseRaw(raw.KindClose)
	r.DurationNs = 812_400_000
	ev, _ := c.Convert(r)
	if ev.EventType != contract.EventClose || ev.DurationMS == nil || *ev.DurationMS != 812.4 {
		t.Fatalf("wrong CLOSE duration: %+v", ev.DurationMS)
	}
}

func TestDrops(t *testing.T) {
	c := newConverter(t)

	r := baseRaw(raw.KindConnect)
	r.DPort = 9200 // not a registered service port
	if _, drop := c.Convert(r); drop != DropUnregisteredPort {
		t.Fatalf("want DropUnregisteredPort, got %v", drop)
	}

	r = baseRaw(raw.KindConnect)
	r.PID = 1 // no such process in the fake /proc
	if _, drop := c.Convert(r); drop != DropUnresolvedSource {
		t.Fatalf("want DropUnresolvedSource, got %v", drop)
	}

	if _, drop := c.Convert(baseRaw(99)); drop != DropUnknownKind {
		t.Fatalf("want DropUnknownKind, got %v", drop)
	}
}

// A service's healthcheck dials its own port on 127.0.0.1. The probe inherits the
// container's SERVICE_NAME, so without a loopback filter every node also reports
// a connection to itself.
func TestDropsLoopback(t *testing.T) {
	c := newConverter(t)

	r := baseRaw(raw.KindConnect)
	r.SAddr = [4]byte{127, 0, 0, 1}
	r.DAddr = [4]byte{127, 0, 0, 1}
	if _, drop := c.Convert(r); drop != DropLoopback {
		t.Fatalf("want DropLoopback, got %v", drop)
	}

	// one side on loopback, the other on the container network: keep it
	r.DAddr = [4]byte{172, 18, 0, 5}
	if _, drop := c.Convert(r); drop != Keep {
		t.Fatalf("non-loopback traffic must be kept, got %v", drop)
	}

	// debugging mode keeps loopback so you can prove the filter is what dropped it
	c.EmitUnresolved = true
	r.DAddr = [4]byte{127, 0, 0, 1}
	if _, drop := c.Convert(r); drop != Keep {
		t.Fatalf("EmitUnresolved must keep loopback, got %v", drop)
	}
}

func TestEmitUnresolvedKeepsEvent(t *testing.T) {
	c := newConverter(t)
	c.EmitUnresolved = true
	r := baseRaw(raw.KindConnect)
	r.PID = 1
	r.DPort = 9200
	ev, drop := c.Convert(r)
	if drop != Keep || ev.SrcService != nil || ev.DstService != nil {
		t.Fatalf("expected kept event with null services, got drop=%v ev=%+v", drop, ev)
	}
}
