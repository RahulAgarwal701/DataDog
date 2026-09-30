package raw

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestLayoutSizeMatchesC(t *testing.T) {
	// sizeof(struct event) in bpf/netmon.c is 56. If this fails, raw.go and netmon.c have drifted.
	if got := binary.Size(Event{}); got != Size {
		t.Fatalf("binary.Size(Event) = %d, want %d", got, Size)
	}
}

func TestDecodeRoundTrip(t *testing.T) {
	in := Event{
		TsNs: 123456789, DurationNs: 42_000_000, PID: 4121, Kind: KindClose,
		SAddr: [4]byte{172, 18, 0, 4}, DAddr: [4]byte{172, 18, 0, 5}, SPort: 51522, DPort: 8002,
	}
	copy(in.Comm[:], "python")
	var buf bytes.Buffer
	if err := binary.Write(&buf, binary.LittleEndian, in); err != nil {
		t.Fatal(err)
	}
	if buf.Len() != Size {
		t.Fatalf("encoded %d bytes, want %d", buf.Len(), Size)
	}
	out, err := Decode(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if out != in {
		t.Fatalf("round trip differs:\n in %+v\nout %+v", in, out)
	}
}

func TestDecodeShortSample(t *testing.T) {
	if _, err := Decode(make([]byte, Size-1)); err == nil {
		t.Fatal("expected error for short sample")
	}
}
