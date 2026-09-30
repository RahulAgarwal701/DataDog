// Package store keeps recent network events in a fixed-size ring buffer and
// derives the service-to-service topology from them.
package store

import (
	"sort"
	"sync"
	"time"

	"github.com/minidd/topology/pkg/contract"
	"github.com/minidd/topology/pkg/registry"
)

// ActiveWithin is how recently an edge must have been seen to be ACTIVE.
const ActiveWithin = 60 * time.Second

type edgeKey struct{ src, dst string }

type edgeSeen struct{ first, last time.Time }

type Store struct {
	mu   sync.RWMutex
	cap  int
	buf  []contract.NetworkEvent
	head int // next write position; when the buffer is full this is also the oldest event
	size int
	ids  map[string]struct{}
	seen map[edgeKey]edgeSeen // all-time first/last seen per edge
	reg  *registry.Registry
}

func New(capacity int, reg *registry.Registry) *Store {
	if capacity < 1 {
		capacity = 1
	}
	return &Store{
		cap:  capacity,
		buf:  make([]contract.NetworkEvent, capacity),
		ids:  make(map[string]struct{}, capacity),
		seen: make(map[edgeKey]edgeSeen),
		reg:  reg,
	}
}

// Len returns the number of buffered events.
func (s *Store) Len() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.size
}

// Add stores events (already validated). Re-sending an event_id is a harmless
// no-op, so agents can safely retry a batch. It returns the number of events
// accepted (duplicates count as accepted).
func (s *Store) Add(events []contract.NetworkEvent) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	accepted := 0
	for _, ev := range events {
		accepted++
		if _, dup := s.ids[ev.EventID]; dup {
			continue
		}
		if s.size == s.cap {
			delete(s.ids, s.buf[s.head].EventID) // evict the oldest
		} else {
			s.size++
		}
		s.buf[s.head] = ev
		s.ids[ev.EventID] = struct{}{}
		s.head = (s.head + 1) % s.cap
		s.trackEdge(ev)
	}
	return accepted
}

func (s *Store) trackEdge(ev contract.NetworkEvent) {
	if ev.EventType != contract.EventConnect && ev.EventType != contract.EventConnectFailed {
		return
	}
	if ev.SrcService == nil || ev.DstService == nil {
		return
	}
	k := edgeKey{*ev.SrcService, *ev.DstService}
	t := ev.Timestamp.Time
	cur, ok := s.seen[k]
	if !ok {
		s.seen[k] = edgeSeen{first: t, last: t}
		return
	}
	if t.Before(cur.first) {
		cur.first = t
	}
	if t.After(cur.last) {
		cur.last = t
	}
	s.seen[k] = cur
}

// Filter selects events. Zero values mean "any".
type Filter struct {
	Service   string // matches src_service or dst_service
	EventType contract.EventType
	Start     *time.Time // inclusive
	End       *time.Time // inclusive
}

func (f Filter) match(ev *contract.NetworkEvent) bool {
	if f.EventType != "" && ev.EventType != f.EventType {
		return false
	}
	if f.Service != "" {
		srcOK := ev.SrcService != nil && *ev.SrcService == f.Service
		dstOK := ev.DstService != nil && *ev.DstService == f.Service
		if !srcOK && !dstOK {
			return false
		}
	}
	if f.Start != nil && ev.Timestamp.Time.Before(*f.Start) {
		return false
	}
	if f.End != nil && ev.Timestamp.Time.After(*f.End) {
		return false
	}
	return true
}

// Query returns one page of matching events, newest first, plus the total match count.
func (s *Store) Query(f Filter, limit, offset int) ([]contract.NetworkEvent, int) {
	s.mu.RLock()
	matches := make([]contract.NetworkEvent, 0, 128)
	for i := 0; i < s.size; i++ {
		idx := (s.head - 1 - i + s.cap) % s.cap
		if f.match(&s.buf[idx]) {
			matches = append(matches, s.buf[idx])
		}
	}
	s.mu.RUnlock()

	sort.SliceStable(matches, func(a, b int) bool {
		return matches[a].Timestamp.Time.After(matches[b].Timestamp.Time)
	})
	total := len(matches)
	if offset >= total {
		return []contract.NetworkEvent{}, total
	}
	end := offset + limit
	if end > total {
		end = total
	}
	return matches[offset:end], total
}

type counts struct{ conn, failed int }

// Graph builds the topology from events inside [now-window, now].
// connection_count / failed_count are counted within the window; first_seen and
// last_seen are all-time. An edge is ACTIVE if last seen within ActiveWithin of now.
func (s *Store) Graph(now time.Time, window time.Duration) contract.TopologyGraph {
	cutoff := now.Add(-window)
	agg := map[edgeKey]*counts{}

	s.mu.RLock()
	for i := 0; i < s.size; i++ {
		ev := &s.buf[(s.head-1-i+s.cap)%s.cap]
		if ev.SrcService == nil || ev.DstService == nil {
			continue
		}
		if ev.EventType != contract.EventConnect && ev.EventType != contract.EventConnectFailed {
			continue
		}
		if ev.Timestamp.Time.Before(cutoff) {
			continue
		}
		k := edgeKey{*ev.SrcService, *ev.DstService}
		c := agg[k]
		if c == nil {
			c = &counts{}
			agg[k] = c
		}
		if ev.EventType == contract.EventConnect {
			c.conn++
		} else {
			c.failed++
		}
	}
	seen := make(map[edgeKey]edgeSeen, len(agg))
	for k := range agg {
		seen[k] = s.seen[k]
	}
	s.mu.RUnlock()

	edges := make([]contract.TopologyEdge, 0, len(agg))
	nodeKind := map[string]string{}
	for _, svc := range s.reg.Services() {
		if svc.Kind == contract.KindService {
			nodeKind[svc.Name] = contract.KindService
		}
	}
	for k, c := range agg {
		sn := seen[k]
		status := contract.EdgeStale
		if now.Sub(sn.last) <= ActiveWithin {
			status = contract.EdgeActive
		}
		edges = append(edges, contract.TopologyEdge{
			Source:          k.src,
			Target:          k.dst,
			FirstSeen:       contract.TS(sn.first),
			LastSeen:        contract.TS(sn.last),
			ConnectionCount: c.conn,
			FailedCount:     c.failed,
			Status:          status,
		})
		for _, id := range []string{k.src, k.dst} {
			if _, ok := nodeKind[id]; !ok {
				nodeKind[id] = s.reg.KindOf(id)
			}
		}
	}
	sort.Slice(edges, func(a, b int) bool {
		if edges[a].Source != edges[b].Source {
			return edges[a].Source < edges[b].Source
		}
		return edges[a].Target < edges[b].Target
	})

	ids := make([]string, 0, len(nodeKind))
	for id := range nodeKind {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	nodes := make([]contract.TopologyNode, 0, len(ids))
	for _, id := range ids {
		nodes = append(nodes, contract.TopologyNode{ID: id, Kind: nodeKind[id]})
	}

	return contract.TopologyGraph{
		GeneratedAt:   contract.TS(now),
		WindowSeconds: int(window / time.Second),
		Nodes:         nodes,
		Edges:         edges,
	}
}
