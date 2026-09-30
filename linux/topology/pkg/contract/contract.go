// Package contract mirrors, in Go, the parts of contracts/python/minidd_contracts/models.py
// (v1.0.0) that the topology service and the eBPF agent produce or consume.
// The pydantic models are the source of truth: if this file and models.py disagree, this file is wrong.
package contract

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"time"
)

const Version = "1.0.0"

// tsLayout is the contract timestamp format: UTC, millisecond precision, trailing Z.
const tsLayout = "2006-01-02T15:04:05.000Z"

// Timestamp is a time.Time that always (un)marshals as 2026-10-14T09:30:01.412Z.
type Timestamp struct{ time.Time }

// TS converts a time.Time to a contract Timestamp (UTC).
func TS(t time.Time) Timestamp { return Timestamp{Time: t.UTC()} }

// Now returns the current time as a contract Timestamp.
func Now() Timestamp { return TS(time.Now()) }

func (t Timestamp) MarshalJSON() ([]byte, error) {
	return []byte(`"` + t.Time.UTC().Format(tsLayout) + `"`), nil
}

func (t *Timestamp) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return fmt.Errorf("timestamp must be a string: %w", err)
	}
	p, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return fmt.Errorf("invalid timestamp %q (want ISO-8601, e.g. 2026-10-14T09:30:01.412Z)", s)
	}
	t.Time = p.UTC()
	return nil
}

// ---------- enums ----------

type EventType string

const (
	EventConnect       EventType = "CONNECT"
	EventAccept        EventType = "ACCEPT"
	EventClose         EventType = "CLOSE"
	EventConnectFailed EventType = "CONNECT_FAILED"
)

func (e EventType) Valid() bool {
	switch e {
	case EventConnect, EventAccept, EventClose, EventConnectFailed:
		return true
	}
	return false
}

type EdgeStatus string

const (
	EdgeActive EdgeStatus = "ACTIVE" // seen in the last 60 s
	EdgeStale  EdgeStatus = "STALE"
)

const (
	KindService  = "service"
	KindExternal = "external"
)

const (
	CodeInvalidArgument     = "invalid_argument"
	CodeNotFound            = "not_found"
	CodeUpstreamUnavailable = "upstream_unavailable"
	CodeInternal            = "internal_error"
)

// ---------- DTOs ----------

type NetworkEvent struct {
	EventID     string    `json:"event_id"`
	Timestamp   Timestamp `json:"timestamp"`
	Host        string    `json:"host"`
	EventType   EventType `json:"event_type"`
	Protocol    string    `json:"protocol"`
	SrcIP       string    `json:"src_ip"`
	SrcPort     int       `json:"src_port"`
	DstIP       string    `json:"dst_ip"`
	DstPort     int       `json:"dst_port"`
	SrcService  *string   `json:"src_service"`
	DstService  *string   `json:"dst_service"`
	PID         int       `json:"pid"`
	ProcessName string    `json:"process_name"`
	DurationMS  *float64  `json:"duration_ms"` // CLOSE only
	Error       *string   `json:"error"`       // CONNECT_FAILED only
}

// Normalize applies contract defaults (protocol defaults to "tcp").
func (e *NetworkEvent) Normalize() {
	if e.Protocol == "" {
		e.Protocol = "tcp"
	}
}

// Validate checks the event against the contract rules.
func (e *NetworkEvent) Validate() error {
	switch {
	case e.EventID == "":
		return errors.New("event_id is required")
	case e.Timestamp.IsZero():
		return errors.New("timestamp is required")
	case e.Host == "":
		return errors.New("host is required")
	case !e.EventType.Valid():
		return fmt.Errorf("invalid event_type %q", e.EventType)
	case e.Protocol != "tcp":
		return fmt.Errorf("protocol must be \"tcp\", got %q", e.Protocol)
	case net.ParseIP(e.SrcIP) == nil:
		return fmt.Errorf("invalid src_ip %q", e.SrcIP)
	case net.ParseIP(e.DstIP) == nil:
		return fmt.Errorf("invalid dst_ip %q", e.DstIP)
	case e.SrcPort < 0 || e.SrcPort > 65535:
		return fmt.Errorf("src_port out of range: %d", e.SrcPort)
	case e.DstPort < 0 || e.DstPort > 65535:
		return fmt.Errorf("dst_port out of range: %d", e.DstPort)
	case e.PID < 0:
		return fmt.Errorf("pid must be >= 0, got %d", e.PID)
	}
	return nil
}

type NetworkEventBatch struct {
	Events []NetworkEvent `json:"events"`
}

type IngestAck struct {
	Accepted int `json:"accepted"`
	Rejected int `json:"rejected"`
}

type TopologyNode struct {
	ID   string `json:"id"`
	Kind string `json:"kind"`
}

type TopologyEdge struct {
	Source          string     `json:"source"`
	Target          string     `json:"target"`
	FirstSeen       Timestamp  `json:"first_seen"`
	LastSeen        Timestamp  `json:"last_seen"`
	ConnectionCount int        `json:"connection_count"` // successful CONNECT events in the window
	FailedCount     int        `json:"failed_count"`     // CONNECT_FAILED events in the window
	Status          EdgeStatus `json:"status"`
}

type TopologyGraph struct {
	GeneratedAt   Timestamp      `json:"generated_at"`
	WindowSeconds int            `json:"window_seconds"`
	Nodes         []TopologyNode `json:"nodes"`
	Edges         []TopologyEdge `json:"edges"`
}

type Page[T any] struct {
	Items  []T `json:"items"`
	Total  int `json:"total"`
	Limit  int `json:"limit"`
	Offset int `json:"offset"`
}

// NewPage builds a Page, guaranteeing items serialises as [] rather than null.
func NewPage[T any](items []T, total, limit, offset int) Page[T] {
	if items == nil {
		items = []T{}
	}
	return Page[T]{Items: items, Total: total, Limit: limit, Offset: offset}
}

type HealthResponse struct {
	Service   string    `json:"service"`
	Status    string    `json:"status"` // "ok" | "degraded"
	Version   string    `json:"version"`
	Timestamp Timestamp `json:"timestamp"`
}

type ErrorBody struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details"`
}

type ErrorResponse struct {
	Error ErrorBody `json:"error"`
}

// ---------- helpers ----------

// Ptr returns a pointer to v (handy for the contract's Optional fields).
func Ptr[T any](v T) *T { return &v }

// NewUUID returns a random RFC 4122 version-4 UUID string.
func NewUUID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// DecodeStrict decodes exactly one JSON value and rejects unknown fields,
// mirroring pydantic's extra="forbid".
func DecodeStrict(r io.Reader, v any) error {
	dec := json.NewDecoder(r)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return err
	}
	if dec.More() {
		return errors.New("unexpected data after JSON value")
	}
	return nil
}
