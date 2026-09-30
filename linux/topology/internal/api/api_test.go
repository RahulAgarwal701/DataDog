package api

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/minidd/topology/internal/store"
	"github.com/minidd/topology/pkg/contract"
	"github.com/minidd/topology/pkg/registry"
)

func newTestServer(t *testing.T) (*httptest.Server, time.Time) {
	t.Helper()
	reg, err := registry.Load("../../testdata/registry.yaml")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 14, 9, 30, 0, 0, time.UTC)
	s := &Server{
		Store:    store.New(1000, reg),
		Log:      slog.New(slog.NewTextHandler(io.Discard, nil)),
		Now:      func() time.Time { return now },
		MaxBatch: 3,
		Version:  "test",
	}
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)
	return ts, now
}

func eventJSON(id, typ string, ts time.Time, src, dst string) string {
	return fmt.Sprintf(`{"event_id":%q,"timestamp":%q,"host":"h","event_type":%q,"protocol":"tcp",
"src_ip":"127.0.0.1","src_port":40000,"dst_ip":"127.0.0.1","dst_port":8002,"src_service":%q,"dst_service":%q,
"pid":1,"process_name":"python","duration_ms":null,"error":null}`,
		id, ts.UTC().Format("2006-01-02T15:04:05.000Z"), typ, src, dst)
}

func post(t *testing.T, url, body string) (*http.Response, []byte) {
	t.Helper()
	resp, err := http.Post(url, "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp, b
}

func get(t *testing.T, url string) (*http.Response, []byte) {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp, b
}

func TestHealth(t *testing.T) {
	ts, _ := newTestServer(t)
	resp, b := get(t, ts.URL+"/health")
	var h contract.HealthResponse
	if resp.StatusCode != 200 || json.Unmarshal(b, &h) != nil || h.Service != "topology-api" || h.Status != "ok" {
		t.Fatalf("bad health: %d %s", resp.StatusCode, b)
	}
}

func TestIngestThenQueryAndTopology(t *testing.T) {
	ts, now := newTestServer(t)
	body := `{"events":[` + eventJSON("1", "CONNECT", now.Add(-2*time.Second), "orders", "payments") + `,` +
		eventJSON("2", "CONNECT_FAILED", now.Add(-1*time.Second), "orders", "payments") + `]}`
	resp, b := post(t, ts.URL+"/api/v1/network-events", body)
	var ack contract.IngestAck
	if resp.StatusCode != 202 || json.Unmarshal(b, &ack) != nil || ack.Accepted != 2 || ack.Rejected != 0 {
		t.Fatalf("ingest: %d %s", resp.StatusCode, b)
	}

	resp, b = get(t, ts.URL+"/api/v1/network-events?service=payments&limit=10")
	var page contract.Page[contract.NetworkEvent]
	if resp.StatusCode != 200 || json.Unmarshal(b, &page) != nil || page.Total != 2 || len(page.Items) != 2 {
		t.Fatalf("list: %d %s", resp.StatusCode, b)
	}
	if page.Items[0].EventID != "2" {
		t.Fatalf("expected newest first, got %s", page.Items[0].EventID)
	}

	resp, b = get(t, ts.URL+"/api/v1/topology?window_seconds=300")
	var g contract.TopologyGraph
	if resp.StatusCode != 200 || json.Unmarshal(b, &g) != nil || len(g.Edges) != 1 {
		t.Fatalf("topology: %d %s", resp.StatusCode, b)
	}
	if g.Edges[0].ConnectionCount != 1 || g.Edges[0].FailedCount != 1 {
		t.Fatalf("edge counts wrong: %+v", g.Edges[0])
	}
}

func TestIngestRejectsBadInput(t *testing.T) {
	ts, now := newTestServer(t)
	u := ts.URL + "/api/v1/network-events"

	resp, b := post(t, u, `{"events":[],"bogus":1}`)
	assertError(t, resp, b, 400, "invalid_argument")

	resp, b = post(t, u, `{}`)
	assertError(t, resp, b, 400, "invalid_argument")

	four := eventJSON("1", "CONNECT", now, "a", "b")
	resp, b = post(t, u, `{"events":[`+four+`,`+four+`,`+four+`,`+four+`]}`)
	assertError(t, resp, b, 400, "invalid_argument") // MaxBatch is 3 in this test server

	// one invalid event (bad type) is counted as rejected, not a 400
	bad := strings.Replace(eventJSON("2", "CONNECT", now, "a", "b"), `"CONNECT"`, `"WHATEVER"`, 1)
	resp, b = post(t, u, `{"events":[`+eventJSON("3", "CONNECT", now, "a", "b")+`,`+bad+`]}`)
	var ack contract.IngestAck
	if resp.StatusCode != 202 || json.Unmarshal(b, &ack) != nil || ack.Accepted != 1 || ack.Rejected != 1 {
		t.Fatalf("partial ingest: %d %s", resp.StatusCode, b)
	}
}

func TestQueryValidation(t *testing.T) {
	ts, _ := newTestServer(t)
	for _, q := range []string{"event_type=NOPE", "limit=0", "limit=5000", "start=yesterday", "offset=-1"} {
		resp, b := get(t, ts.URL+"/api/v1/network-events?"+q)
		assertError(t, resp, b, 400, "invalid_argument")
	}
	resp, b := get(t, ts.URL+"/api/v1/topology?window_seconds=0")
	assertError(t, resp, b, 400, "invalid_argument")
	resp, b = get(t, ts.URL+"/nope")
	assertError(t, resp, b, 404, "not_found")
}

func TestEmptyListIsArrayNotNull(t *testing.T) {
	ts, _ := newTestServer(t)
	_, b := get(t, ts.URL+"/api/v1/network-events")
	if !strings.Contains(string(b), `"items":[]`) {
		t.Fatalf("items must be [] when empty: %s", b)
	}
	_, b = get(t, ts.URL+"/api/v1/topology")
	if !strings.Contains(string(b), `"edges":[]`) {
		t.Fatalf("edges must be [] when empty: %s", b)
	}
}

func assertError(t *testing.T, resp *http.Response, body []byte, status int, code string) {
	t.Helper()
	var e contract.ErrorResponse
	if resp.StatusCode != status || json.Unmarshal(body, &e) != nil || e.Error.Code != code {
		t.Fatalf("want %d/%s, got %d %s", status, code, resp.StatusCode, body)
	}
}
