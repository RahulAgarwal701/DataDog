package contract

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"
)

// Fixtures copied verbatim from 02_CONTRACTS.md section 8.
const fixtureConnect = `{"event_id":"6f1c2a7e-3b0d-4d7e-9a55-0c2f1d8e4b10","timestamp":"2026-10-14T09:30:01.410Z","host":"demo-vm",
 "event_type":"CONNECT","protocol":"tcp","src_ip":"172.18.0.4","src_port":51522,"dst_ip":"172.18.0.5","dst_port":8002,
 "src_service":"orders","dst_service":"payments","pid":4121,"process_name":"python","duration_ms":null,"error":null}`

const fixtureFailed = `{"event_id":"a91b0c33-52de-4a88-b7e1-99d3c6a2f0aa","timestamp":"2026-10-14T09:31:14.020Z","host":"demo-vm",
 "event_type":"CONNECT_FAILED","protocol":"tcp","src_ip":"172.18.0.4","src_port":51610,"dst_ip":"172.18.0.5","dst_port":8002,
 "src_service":"orders","dst_service":"payments","pid":4121,"process_name":"python","duration_ms":null,"error":"ECONNREFUSED"}`

func TestTimestampFormat(t *testing.T) {
	ts := TS(time.Date(2026, 10, 14, 9, 30, 1, 412_000_000, time.UTC))
	b, err := json.Marshal(ts)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != `"2026-10-14T09:30:01.412Z"` {
		t.Fatalf("got %s", b)
	}
	// whole seconds must still carry .000
	b, _ = json.Marshal(TS(time.Date(2026, 10, 14, 9, 30, 1, 0, time.UTC)))
	if string(b) != `"2026-10-14T09:30:01.000Z"` {
		t.Fatalf("got %s", b)
	}
}

func TestTimestampNormalisesOffsetToUTC(t *testing.T) {
	var ts Timestamp
	if err := json.Unmarshal([]byte(`"2026-10-14T15:00:01.500+05:30"`), &ts); err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(ts)
	if string(b) != `"2026-10-14T09:30:01.500Z"` {
		t.Fatalf("got %s", b)
	}
	if err := json.Unmarshal([]byte(`"yesterday"`), &ts); err == nil {
		t.Fatal("expected error for bad timestamp")
	}
}

func TestFixturesRoundTrip(t *testing.T) {
	for name, fx := range map[string]string{"connect": fixtureConnect, "failed": fixtureFailed} {
		var ev NetworkEvent
		if err := DecodeStrict(strings.NewReader(fx), &ev); err != nil {
			t.Fatalf("%s: decode: %v", name, err)
		}
		ev.Normalize()
		if err := ev.Validate(); err != nil {
			t.Fatalf("%s: validate: %v", name, err)
		}
		out, err := json.Marshal(ev)
		if err != nil {
			t.Fatal(err)
		}
		var want, got map[string]any
		_ = json.Unmarshal([]byte(fx), &want)
		_ = json.Unmarshal(out, &got)
		if !reflect.DeepEqual(want, got) {
			t.Fatalf("%s: round trip differs\nwant %v\ngot  %v", name, want, got)
		}
	}
}

func TestDecodeStrictRejectsUnknownField(t *testing.T) {
	var ev NetworkEvent
	err := DecodeStrict(strings.NewReader(`{"event_id":"x","bogus":1}`), &ev)
	if err == nil {
		t.Fatal("expected unknown field error")
	}
}

func TestValidate(t *testing.T) {
	var ev NetworkEvent
	_ = DecodeStrict(strings.NewReader(fixtureConnect), &ev)
	ev.Normalize()
	bad := ev
	bad.EventType = "NOPE"
	if bad.Validate() == nil {
		t.Fatal("bad event_type accepted")
	}
	bad = ev
	bad.DstIP = "not-an-ip"
	if bad.Validate() == nil {
		t.Fatal("bad dst_ip accepted")
	}
	bad = ev
	bad.DstPort = 70000
	if bad.Validate() == nil {
		t.Fatal("bad port accepted")
	}
}

func TestNewUUID(t *testing.T) {
	a, b := NewUUID(), NewUUID()
	if a == b || len(a) != 36 || a[14] != '4' {
		t.Fatalf("unexpected uuids %q %q", a, b)
	}
}
