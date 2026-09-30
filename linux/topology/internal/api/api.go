// Package api implements the Topology API (contract section 7.4).
package api

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/minidd/topology/internal/store"
	"github.com/minidd/topology/pkg/contract"
)

type Server struct {
	Store    *store.Store
	Log      *slog.Logger
	Now      func() time.Time
	MaxBatch int
	Version  string
}

func (s *Server) Handler() http.Handler {
	if s.Now == nil {
		s.Now = time.Now
	}
	if s.MaxBatch <= 0 {
		s.MaxBatch = 500
	}
	if s.Version == "" {
		s.Version = contract.Version
	}
	if s.Log == nil {
		s.Log = slog.Default()
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", s.health)
	mux.HandleFunc("POST /api/v1/network-events", s.ingest)
	mux.HandleFunc("GET /api/v1/network-events", s.listEvents)
	mux.HandleFunc("GET /api/v1/topology", s.topology)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, contract.CodeNotFound, "no such endpoint: "+r.Method+" "+r.URL.Path)
	})
	return s.recoverer(cors(s.logging(mux)))
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, contract.HealthResponse{
		Service:   "topology-api",
		Status:    "ok",
		Version:   s.Version,
		Timestamp: contract.TS(s.Now()),
	})
}

// ingest handles POST /api/v1/network-events. Malformed JSON, unknown fields or an
// oversized batch are a 400; individually invalid events are counted as rejected.
func (s *Server) ingest(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 2<<20)
	var batch contract.NetworkEventBatch
	if err := contract.DecodeStrict(r.Body, &batch); err != nil {
		writeError(w, http.StatusBadRequest, contract.CodeInvalidArgument, "invalid request body: "+err.Error())
		return
	}
	if batch.Events == nil {
		writeError(w, http.StatusBadRequest, contract.CodeInvalidArgument, "events is required")
		return
	}
	if len(batch.Events) > s.MaxBatch {
		writeError(w, http.StatusBadRequest, contract.CodeInvalidArgument,
			fmt.Sprintf("batch too large: %d events (max %d)", len(batch.Events), s.MaxBatch))
		return
	}
	valid := make([]contract.NetworkEvent, 0, len(batch.Events))
	rejected := 0
	for i := range batch.Events {
		e := &batch.Events[i]
		e.Normalize()
		if err := e.Validate(); err != nil {
			rejected++
			s.Log.Warn("rejected event", "index", i, "reason", err.Error())
			continue
		}
		valid = append(valid, *e)
	}
	accepted := s.Store.Add(valid)
	writeJSON(w, http.StatusAccepted, contract.IngestAck{Accepted: accepted, Rejected: rejected})
}

func (s *Server) listEvents(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, err := intParam(q, "limit", 100, 1, 1000)
	if err != nil {
		writeError(w, http.StatusBadRequest, contract.CodeInvalidArgument, err.Error())
		return
	}
	offset, err := intParam(q, "offset", 0, 0, 1<<30)
	if err != nil {
		writeError(w, http.StatusBadRequest, contract.CodeInvalidArgument, err.Error())
		return
	}
	start, err := timeParam(q, "start")
	if err != nil {
		writeError(w, http.StatusBadRequest, contract.CodeInvalidArgument, err.Error())
		return
	}
	end, err := timeParam(q, "end")
	if err != nil {
		writeError(w, http.StatusBadRequest, contract.CodeInvalidArgument, err.Error())
		return
	}
	et := contract.EventType(q.Get("event_type"))
	if et != "" && !et.Valid() {
		writeError(w, http.StatusBadRequest, contract.CodeInvalidArgument,
			"event_type must be one of CONNECT, ACCEPT, CLOSE, CONNECT_FAILED")
		return
	}
	items, total := s.Store.Query(store.Filter{
		Service:   q.Get("service"),
		EventType: et,
		Start:     start,
		End:       end,
	}, limit, offset)
	writeJSON(w, http.StatusOK, contract.NewPage(items, total, limit, offset))
}

func (s *Server) topology(w http.ResponseWriter, r *http.Request) {
	window, err := intParam(r.URL.Query(), "window_seconds", 300, 1, 86400)
	if err != nil {
		writeError(w, http.StatusBadRequest, contract.CodeInvalidArgument, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, s.Store.Graph(s.Now(), time.Duration(window)*time.Second))
}

// ---------- helpers ----------

func intParam(q url.Values, name string, def, min, max int) (int, error) {
	v := q.Get(name)
	if v == "" {
		return def, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < min || n > max {
		return 0, fmt.Errorf("%s must be an integer between %d and %d", name, min, max)
	}
	return n, nil
}

func timeParam(q url.Values, name string) (*time.Time, error) {
	v := q.Get(name)
	if v == "" {
		return nil, nil
	}
	t, err := time.Parse(time.RFC3339Nano, v)
	if err != nil {
		return nil, fmt.Errorf("%s must be an ISO-8601 timestamp, e.g. 2026-10-14T09:30:00.000Z", name)
	}
	t = t.UTC()
	return &t, nil
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, code, msg string) {
	writeJSON(w, status, contract.ErrorResponse{Error: contract.ErrorBody{Code: code, Message: msg}})
}

func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Access-Control-Allow-Origin", "*")
		h.Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		h.Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func (s *Server) logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		if r.URL.Path == "/health" {
			return
		}
		s.Log.Info("request", "method", r.Method, "path", r.URL.Path,
			"status", rec.status, "duration_ms", time.Since(start).Milliseconds())
	})
}

func (s *Server) recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				s.Log.Error("panic in handler", "panic", fmt.Sprint(rec))
				writeError(w, http.StatusInternalServerError, contract.CodeInternal, "internal error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}
