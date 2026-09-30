// Package mock produces synthetic network events (MOCK=true) so the dashboard can be
// built and demoed without a Linux/eBPF host.
package mock

import (
	"context"
	"math/rand"
	"time"

	"github.com/minidd/topology/internal/store"
	"github.com/minidd/topology/pkg/contract"
	"github.com/minidd/topology/pkg/registry"
)

func pairs(reg *registry.Registry) [][2]registry.Service {
	var out [][2]registry.Service
	for _, s := range reg.Services() {
		for _, d := range s.DependsOn {
			if dst, ok := reg.ByName(d); ok {
				out = append(out, [2]registry.Service{s, dst})
			}
		}
	}
	return out
}

func event(src, dst registry.Service, ts time.Time, typ contract.EventType) contract.NetworkEvent {
	e := contract.NetworkEvent{
		EventID:     contract.NewUUID(),
		Timestamp:   contract.TS(ts),
		Host:        "mock-host",
		EventType:   typ,
		Protocol:    "tcp",
		SrcIP:       "127.0.0.1",
		SrcPort:     32768 + rand.Intn(28000),
		DstIP:       "127.0.0.1",
		DstPort:     dst.Port,
		SrcService:  contract.Ptr(src.Name),
		DstService:  contract.Ptr(dst.Name),
		PID:         1000 + rand.Intn(100),
		ProcessName: "python",
	}
	switch typ {
	case contract.EventClose:
		e.DurationMS = contract.Ptr(float64(20 + rand.Intn(80)))
	case contract.EventConnectFailed:
		e.Error = contract.Ptr("ECONNREFUSED")
	}
	return e
}

// Seed back-fills `span` of history ending at now (one connect+close per edge per second).
func Seed(st *store.Store, reg *registry.Registry, now time.Time, span time.Duration) {
	ps := pairs(reg)
	var batch []contract.NetworkEvent
	for t := now.Add(-span); t.Before(now); t = t.Add(time.Second) {
		for _, p := range ps {
			batch = append(batch, event(p[0], p[1], t, contract.EventConnect))
			batch = append(batch, event(p[0], p[1], t.Add(80*time.Millisecond), contract.EventClose))
		}
	}
	st.Add(batch)
}

// Run emits live events every second until ctx is cancelled. For a few seconds of
// every minute one edge shows CONNECT_FAILED events so the UI has something red to show.
func Run(ctx context.Context, st *store.Store, reg *registry.Registry) {
	ps := pairs(reg)
	if len(ps) == 0 {
		return
	}
	failing := ps[len(ps)/2]
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	n := 0
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-tick.C:
			n++
			var batch []contract.NetworkEvent
			for _, p := range ps {
				isFailing := p[0].Name == failing[0].Name && p[1].Name == failing[1].Name
				if isFailing && n%60 >= 30 && n%60 < 35 {
					for i := 0; i < 5; i++ {
						batch = append(batch, event(p[0], p[1], now, contract.EventConnectFailed))
					}
					continue
				}
				for i := 0; i < 5; i++ {
					batch = append(batch, event(p[0], p[1], now, contract.EventConnect))
					batch = append(batch, event(p[0], p[1], now.Add(60*time.Millisecond), contract.EventClose))
				}
			}
			st.Add(batch)
		}
	}
}
