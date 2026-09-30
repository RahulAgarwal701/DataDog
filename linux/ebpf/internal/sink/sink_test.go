package sink

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/minidd/topology/pkg/contract"
)

func ev(id string) contract.NetworkEvent {
	return contract.NetworkEvent{
		EventID: id, Timestamp: contract.Now(), Host: "h", EventType: contract.EventConnect, Protocol: "tcp",
		SrcIP: "127.0.0.1", DstIP: "127.0.0.1", DstPort: 8002, PID: 1, ProcessName: "x",
	}
}

func TestFlushOnceSuccess(t *testing.T) {
	var got atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/network-events" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		var b contract.NetworkEventBatch
		if err := contract.DecodeStrict(r.Body, &b); err != nil {
			t.Errorf("agent sent a body the API would reject: %v", err)
		}
		got.Add(int64(len(b.Events)))
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()

	s := New(srv.URL, 2, 0, 100, nil)
	s.Enqueue(ev("1"))
	s.Enqueue(ev("2"))
	s.Enqueue(ev("3"))
	if err := s.FlushOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got.Load() != 2 || s.Pending() != 1 || s.Sent.Load() != 2 {
		t.Fatalf("got=%d pending=%d sent=%d", got.Load(), s.Pending(), s.Sent.Load())
	}
}

func TestServerErrorKeepsBatchQueued(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	s := New(srv.URL, 10, 0, 100, nil)
	s.Enqueue(ev("1"))
	if err := s.FlushOnce(context.Background()); err == nil {
		t.Fatal("expected error on 500")
	}
	if s.Pending() != 1 {
		t.Fatalf("batch must stay queued, pending=%d", s.Pending())
	}
}

func TestClientErrorDropsBatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer srv.Close()
	s := New(srv.URL, 10, 0, 100, nil)
	s.Enqueue(ev("1"))
	if err := s.FlushOnce(context.Background()); err == nil {
		t.Fatal("expected error on 400")
	}
	if s.Pending() != 0 || s.Dropped.Load() != 1 {
		t.Fatalf("pending=%d dropped=%d", s.Pending(), s.Dropped.Load())
	}
}

func TestQueueOverflowDropsOldest(t *testing.T) {
	s := New("http://127.0.0.1:1", 2, 0, 2, nil)
	s.Enqueue(ev("1"))
	s.Enqueue(ev("2"))
	s.Enqueue(ev("3"))
	if s.Pending() != 2 || s.Dropped.Load() != 1 {
		t.Fatalf("pending=%d dropped=%d", s.Pending(), s.Dropped.Load())
	}
}

func TestBackoff(t *testing.T) {
	d := nextBackoff(0)
	for i := 0; i < 10; i++ {
		d = nextBackoff(d)
	}
	if d != 5_000_000_000 {
		t.Fatalf("backoff should cap at 5s, got %s", d)
	}
}
