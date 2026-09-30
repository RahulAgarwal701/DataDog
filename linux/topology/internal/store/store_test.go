package store

import (
	"testing"
	"time"

	"github.com/minidd/topology/pkg/contract"
	"github.com/minidd/topology/pkg/registry"
)

func testRegistry(t *testing.T) *registry.Registry {
	t.Helper()
	r, err := registry.Load("../../testdata/registry.yaml")
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func ev(id string, typ contract.EventType, ts time.Time, src, dst string) contract.NetworkEvent {
	e := contract.NetworkEvent{
		EventID: id, Timestamp: contract.TS(ts), Host: "h", EventType: typ, Protocol: "tcp",
		SrcIP: "127.0.0.1", SrcPort: 40000, DstIP: "127.0.0.1", DstPort: 8002, PID: 1, ProcessName: "x",
	}
	if src != "" {
		e.SrcService = contract.Ptr(src)
	}
	if dst != "" {
		e.DstService = contract.Ptr(dst)
	}
	return e
}

func TestAddDedupAndEviction(t *testing.T) {
	st := New(3, testRegistry(t))
	now := time.Now()
	st.Add([]contract.NetworkEvent{
		ev("1", contract.EventConnect, now, "orders", "payments"),
		ev("1", contract.EventConnect, now, "orders", "payments"), // duplicate
		ev("2", contract.EventConnect, now, "orders", "payments"),
	})
	if st.Len() != 2 {
		t.Fatalf("want 2 stored, got %d", st.Len())
	}
	st.Add([]contract.NetworkEvent{
		ev("3", contract.EventConnect, now, "orders", "payments"),
		ev("4", contract.EventConnect, now, "orders", "payments"), // evicts "1"
	})
	if st.Len() != 3 {
		t.Fatalf("want 3 stored, got %d", st.Len())
	}
	// "1" was evicted, so it may be stored again
	st.Add([]contract.NetworkEvent{ev("1", contract.EventConnect, now, "orders", "payments")})
	items, total := st.Query(Filter{}, 100, 0)
	if total != 3 || len(items) != 3 {
		t.Fatalf("total=%d len=%d", total, len(items))
	}
}

func TestQueryFilterOrderAndPaging(t *testing.T) {
	st := New(100, testRegistry(t))
	base := time.Date(2026, 10, 14, 9, 0, 0, 0, time.UTC)
	st.Add([]contract.NetworkEvent{
		ev("a", contract.EventConnect, base.Add(1*time.Second), "orders", "payments"),
		ev("b", contract.EventConnectFailed, base.Add(2*time.Second), "orders", "payments"),
		ev("c", contract.EventConnect, base.Add(3*time.Second), "api-gateway", "orders"),
	})
	items, total := st.Query(Filter{}, 100, 0)
	if total != 3 || items[0].EventID != "c" || items[2].EventID != "a" {
		t.Fatalf("not newest-first: %+v", items)
	}
	_, total = st.Query(Filter{Service: "payments"}, 100, 0)
	if total != 2 {
		t.Fatalf("service filter matched %d, want 2", total)
	}
	_, total = st.Query(Filter{EventType: contract.EventConnectFailed}, 100, 0)
	if total != 1 {
		t.Fatalf("type filter matched %d, want 1", total)
	}
	start := base.Add(2 * time.Second)
	_, total = st.Query(Filter{Start: &start}, 100, 0)
	if total != 2 {
		t.Fatalf("start filter matched %d, want 2", total)
	}
	page, total := st.Query(Filter{}, 1, 1)
	if total != 3 || len(page) != 1 || page[0].EventID != "b" {
		t.Fatalf("paging wrong: total=%d page=%+v", total, page)
	}
	empty, _ := st.Query(Filter{}, 10, 50)
	if empty == nil || len(empty) != 0 {
		t.Fatal("offset past end must give empty, non-nil slice")
	}
}

func TestGraph(t *testing.T) {
	st := New(1000, testRegistry(t))
	now := time.Date(2026, 10, 14, 9, 30, 0, 0, time.UTC)
	st.Add([]contract.NetworkEvent{
		ev("1", contract.EventConnect, now.Add(-10*time.Second), "orders", "payments"),
		ev("2", contract.EventConnect, now.Add(-5*time.Second), "orders", "payments"),
		ev("3", contract.EventConnectFailed, now.Add(-2*time.Second), "orders", "payments"),
		ev("4", contract.EventConnect, now.Add(-200*time.Second), "api-gateway", "orders"), // STALE (>60s) but in window
		ev("5", contract.EventConnect, now.Add(-1*time.Second), "loadgen", "api-gateway"),
		ev("6", contract.EventClose, now, "orders", "payments"),               // CLOSE never creates edges
		ev("7", contract.EventConnect, now, "", "payments"),                    // unresolved source: ignored
		ev("8", contract.EventConnect, now.Add(-1000*time.Second), "payments", "inventory"), // outside 300s window
	})
	g := st.Graph(now, 300*time.Second)
	if g.WindowSeconds != 300 {
		t.Fatalf("window_seconds = %d", g.WindowSeconds)
	}
	if len(g.Edges) != 3 {
		t.Fatalf("want 3 edges, got %d: %+v", len(g.Edges), g.Edges)
	}
	byKey := map[string]contract.TopologyEdge{}
	for _, e := range g.Edges {
		byKey[e.Source+">"+e.Target] = e
	}
	op := byKey["orders>payments"]
	if op.ConnectionCount != 2 || op.FailedCount != 1 || op.Status != contract.EdgeActive {
		t.Fatalf("orders>payments wrong: %+v", op)
	}
	if byKey["api-gateway>orders"].Status != contract.EdgeStale {
		t.Fatalf("api-gateway>orders should be STALE: %+v", byKey["api-gateway>orders"])
	}
	kinds := map[string]string{}
	for _, n := range g.Nodes {
		kinds[n.ID] = n.Kind
	}
	// all four registry services appear (inventory has no edge in window) + loadgen as external
	for _, id := range []string{"api-gateway", "orders", "payments", "inventory"} {
		if kinds[id] != "service" {
			t.Fatalf("node %s missing or wrong kind: %v", id, kinds)
		}
	}
	if kinds["loadgen"] != "external" {
		t.Fatalf("loadgen must be an external node: %v", kinds)
	}
}
