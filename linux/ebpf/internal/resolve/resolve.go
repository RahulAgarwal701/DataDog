// Package resolve maps raw connection endpoints to service names.
//
//	destination: the destination TCP port is looked up in the service registry
//	source:      the connecting process (PID) is identified by the SERVICE_NAME
//	             environment variable read from /proc/<pid>/environ; if absent, by matching
//	             "services.<name>" / "services/<name>" in /proc/<pid>/cmdline.
//
// Resolution happens in userspace right after the event is read, so a process that exits
// within microseconds of connect() may be unresolvable; those events are counted, not guessed.
package resolve

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/minidd/topology/pkg/registry"
)

const (
	positiveTTL = 60 * time.Second
	negativeTTL = 2 * time.Second
	maxCache    = 4096
)

type entry struct {
	name string
	ok   bool
	at   time.Time
}

type Resolver struct {
	reg      *registry.Registry
	procRoot string
	now      func() time.Time

	mu    sync.Mutex
	cache map[uint32]entry
}

// New creates a Resolver. procRoot is normally "/proc".
func New(reg *registry.Registry, procRoot string) *Resolver {
	return &Resolver{reg: reg, procRoot: procRoot, now: time.Now, cache: map[uint32]entry{}}
}

// Dst resolves a destination port to a registered service name.
func (r *Resolver) Dst(port int) (string, bool) {
	svc, ok := r.reg.ByPort(port)
	if !ok {
		return "", false
	}
	return svc.Name, true
}

// Src resolves the connecting process to a service name.
func (r *Resolver) Src(pid uint32) (string, bool) {
	now := r.now()
	r.mu.Lock()
	if e, hit := r.cache[pid]; hit {
		ttl := negativeTTL
		if e.ok {
			ttl = positiveTTL
		}
		if now.Sub(e.at) < ttl {
			r.mu.Unlock()
			return e.name, e.ok
		}
	}
	r.mu.Unlock()

	name, ok := r.lookup(pid)

	r.mu.Lock()
	if len(r.cache) >= maxCache {
		r.cache = map[uint32]entry{}
	}
	r.cache[pid] = entry{name: name, ok: ok, at: now}
	r.mu.Unlock()
	return name, ok
}

func (r *Resolver) lookup(pid uint32) (string, bool) {
	dir := filepath.Join(r.procRoot, strconv.FormatUint(uint64(pid), 10))

	if data, err := os.ReadFile(filepath.Join(dir, "environ")); err == nil {
		for _, kv := range bytes.Split(data, []byte{0}) {
			if v, found := strings.CutPrefix(string(kv), "SERVICE_NAME="); found && v != "" {
				return v, true
			}
		}
	}

	if data, err := os.ReadFile(filepath.Join(dir, "cmdline")); err == nil {
		cmd := strings.ReplaceAll(string(data), "\x00", " ")
		for _, svc := range r.reg.Services() {
			under := strings.ReplaceAll(svc.Name, "-", "_")
			for _, pat := range []string{"services." + under, "services/" + under, "services." + svc.Name, "services/" + svc.Name} {
				if strings.Contains(cmd, pat) {
					return svc.Name, true
				}
			}
		}
	}
	return "", false
}
