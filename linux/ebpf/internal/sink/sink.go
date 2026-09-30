// Package sink batches NetworkEvents and POSTs them to the Topology API with retry.
package sink

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/minidd/topology/pkg/contract"
)

const maxBatch = 500 // Topology API limit

type Sender struct {
	url      string
	client   *http.Client
	batch    int
	interval time.Duration
	maxQueue int
	logf     func(format string, args ...any)

	mu    sync.Mutex
	queue []contract.NetworkEvent

	Sent     atomic.Uint64 // events accepted by the API
	Failures atomic.Uint64 // failed POST attempts (network error / 5xx)
	Dropped  atomic.Uint64 // events discarded (queue overflow or permanently rejected batch)
}

// New creates a Sender. baseURL is e.g. http://localhost:9003.
func New(baseURL string, batch int, interval time.Duration, maxQueue int, logf func(string, ...any)) *Sender {
	if batch < 1 || batch > maxBatch {
		batch = 200
	}
	if maxQueue < batch {
		maxQueue = batch
	}
	if logf == nil {
		logf = func(string, ...any) {}
	}
	return &Sender{
		url:      strings.TrimRight(baseURL, "/") + "/api/v1/network-events",
		client:   &http.Client{Timeout: 3 * time.Second},
		batch:    batch,
		interval: interval,
		maxQueue: maxQueue,
		logf:     logf,
	}
}

// Enqueue adds an event; if the queue is full the oldest event is dropped.
func (s *Sender) Enqueue(ev contract.NetworkEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.queue) >= s.maxQueue {
		s.queue = s.queue[1:]
		s.Dropped.Add(1)
	}
	s.queue = append(s.queue, ev)
}

// Pending returns the number of queued events.
func (s *Sender) Pending() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.queue)
}

// FlushOnce sends one batch. On success the batch is removed from the queue. On a 4xx
// response the batch is dropped (retrying cannot help); on network errors and 5xx it stays queued.
func (s *Sender) FlushOnce(ctx context.Context) error {
	s.mu.Lock()
	n := len(s.queue)
	if n == 0 {
		s.mu.Unlock()
		return nil
	}
	if n > s.batch {
		n = s.batch
	}
	batch := make([]contract.NetworkEvent, n)
	copy(batch, s.queue[:n])
	s.mu.Unlock()

	body, err := json.Marshal(contract.NetworkEventBatch{Events: batch})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))

	switch {
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		s.remove(n)
		s.Sent.Add(uint64(n))
		return nil
	case resp.StatusCode >= 400 && resp.StatusCode < 500:
		s.remove(n)
		s.Dropped.Add(uint64(n))
		return fmt.Errorf("batch of %d rejected permanently: HTTP %d %s", n, resp.StatusCode, strings.TrimSpace(string(respBody)))
	default:
		return fmt.Errorf("topology API returned HTTP %d", resp.StatusCode)
	}
}

func (s *Sender) remove(n int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if n > len(s.queue) {
		n = len(s.queue)
	}
	s.queue = s.queue[n:]
}

// Run flushes on every tick, with exponential backoff (max 5s) while the API is unreachable.
// On ctx cancellation it makes a final short attempt to drain the queue.
func (s *Sender) Run(ctx context.Context) {
	t := time.NewTicker(s.interval)
	defer t.Stop()
	var backoff time.Duration
	var next time.Time
	for {
		select {
		case <-ctx.Done():
			fctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			for s.Pending() > 0 {
				if err := s.FlushOnce(fctx); err != nil {
					break
				}
			}
			cancel()
			return
		case <-t.C:
			if time.Now().Before(next) {
				continue
			}
			for s.Pending() > 0 {
				if err := s.FlushOnce(ctx); err != nil {
					s.Failures.Add(1)
					backoff = nextBackoff(backoff)
					next = time.Now().Add(backoff)
					s.logf("send failed (retry in %s, %d queued): %v", backoff, s.Pending(), err)
					break
				}
				backoff = 0
			}
		}
	}
}

func nextBackoff(cur time.Duration) time.Duration {
	if cur <= 0 {
		return 500 * time.Millisecond
	}
	cur *= 2
	if cur > 5*time.Second {
		cur = 5 * time.Second
	}
	return cur
}
