package resolve

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/minidd/topology/pkg/registry"
)

const regYAML = `
services:
  - {name: api-gateway, port: 8000, kind: service}
  - {name: orders,      port: 8001, kind: service}
  - {name: payments,    port: 8002, kind: service}
  - {name: loadgen,     port: 8010, kind: external}
`

func newResolver(t *testing.T) (*Resolver, string) {
	t.Helper()
	reg, err := registry.Parse([]byte(regYAML))
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	return New(reg, root), root
}

func writeProc(t *testing.T, root string, pid uint32, environ, cmdline string) {
	t.Helper()
	dir := filepath.Join(root, strconv.Itoa(int(pid)))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if environ != "" {
		if err := os.WriteFile(filepath.Join(dir, "environ"), []byte(environ), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if cmdline != "" {
		if err := os.WriteFile(filepath.Join(dir, "cmdline"), []byte(cmdline), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestDst(t *testing.T) {
	r, _ := newResolver(t)
	if n, ok := r.Dst(8002); !ok || n != "payments" {
		t.Fatalf("Dst(8002) = %q, %v", n, ok)
	}
	if _, ok := r.Dst(9200); ok {
		t.Fatal("unregistered port must not resolve")
	}
}

func TestSrcFromEnviron(t *testing.T) {
	r, root := newResolver(t)
	writeProc(t, root, 4121, "PATH=/usr/bin\x00SERVICE_NAME=orders\x00HOME=/root\x00", "")
	if n, ok := r.Src(4121); !ok || n != "orders" {
		t.Fatalf("Src = %q, %v", n, ok)
	}
}

func TestSrcFallsBackToCmdline(t *testing.T) {
	r, root := newResolver(t)
	writeProc(t, root, 77, "PATH=/usr/bin\x00", "python\x00-m\x00services.api_gateway.main\x00")
	if n, ok := r.Src(77); !ok || n != "api-gateway" {
		t.Fatalf("Src = %q, %v", n, ok)
	}
}

func TestSrcUnresolvedAndNegativeCache(t *testing.T) {
	r, root := newResolver(t)
	now := time.Date(2026, 10, 14, 9, 0, 0, 0, time.UTC)
	r.now = func() time.Time { return now }

	if _, ok := r.Src(999); ok {
		t.Fatal("missing process must not resolve")
	}
	writeProc(t, root, 999, "SERVICE_NAME=payments\x00", "")
	if _, ok := r.Src(999); ok {
		t.Fatal("negative result should be cached briefly")
	}
	now = now.Add(negativeTTL + time.Millisecond)
	if n, ok := r.Src(999); !ok || n != "payments" {
		t.Fatalf("after negative TTL: %q, %v", n, ok)
	}
}
