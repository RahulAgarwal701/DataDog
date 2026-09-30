package pretty

import (
	"strings"
	"testing"
	"time"

	"github.com/minidd/topology/pkg/contract"
)

func failedEvent() contract.NetworkEvent {
	return contract.NetworkEvent{
		EventID: "x", Timestamp: contract.TS(time.Date(2026, 10, 14, 9, 31, 14, 20_000_000, time.UTC)),
		Host: "h", EventType: contract.EventConnectFailed, Protocol: "tcp",
		SrcIP: "127.0.0.1", DstIP: "127.0.0.1", DstPort: 8002, PID: 4121, ProcessName: "python",
		SrcService: contract.Ptr("orders"), DstService: contract.Ptr("payments"), Error: contract.Ptr("ECONNREFUSED"),
	}
}

func TestFormatFailedPlain(t *testing.T) {
	line := New(&strings.Builder{}, false).Format(failedEvent())
	for _, want := range []string{"CONNECT_FAILED", "orders(4121)", "payments:8002", "ECONNREFUSED"} {
		if !strings.Contains(line, want) {
			t.Fatalf("%q missing from %q", want, line)
		}
	}
	if strings.Contains(line, "\x1b") {
		t.Fatalf("colour codes present with colour disabled: %q", line)
	}
}

func TestFormatColour(t *testing.T) {
	line := New(&strings.Builder{}, true).Format(failedEvent())
	if !strings.HasPrefix(line, "\x1b[31;1m") || !strings.HasSuffix(line, "\x1b[0m") {
		t.Fatalf("expected red ANSI wrapping, got %q", line)
	}
}

func TestFormatCloseShowsLifetime(t *testing.T) {
	ev := failedEvent()
	ev.EventType = contract.EventClose
	ev.Error = nil
	ev.DurationMS = contract.Ptr(812.4)
	if line := New(&strings.Builder{}, false).Format(ev); !strings.Contains(line, "lived 812.4ms") {
		t.Fatalf("lifetime missing: %q", line)
	}
}

func TestPrintWritesLine(t *testing.T) {
	var sb strings.Builder
	New(&sb, false).Print(failedEvent())
	if !strings.HasSuffix(sb.String(), "\n") {
		t.Fatalf("Print must end with newline: %q", sb.String())
	}
}
