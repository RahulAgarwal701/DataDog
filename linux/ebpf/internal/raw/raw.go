// Package raw defines the fixed-layout record the eBPF program writes into its ring buffer.
// It MUST stay byte-for-byte in sync with `struct event` in bpf/netmon.c (56 bytes, little endian).
package raw

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

// Event kinds (mirror enum values in netmon.c).
const (
	KindConnect       = 1
	KindConnectFailed = 2
	KindClose         = 3
)

// Size is sizeof(struct event) in C.
const Size = 56

type Event struct {
	TsNs       uint64   // bpf_ktime_get_ns() (CLOCK_MONOTONIC) when the transition happened
	DurationNs uint64   // CONNECT: handshake time; CONNECT_FAILED: time until failure; CLOSE: connection lifetime
	PID        uint32   // tgid of the process that called connect()
	Kind       uint32   // KindConnect | KindConnectFailed | KindClose
	SAddr      [4]byte  // IPv4, network order (already in dotted order when read as bytes)
	DAddr      [4]byte  //
	SPort      uint16   // host byte order
	DPort      uint16   // host byte order
	Comm       [16]byte // process name, NUL padded
	_          uint32   // explicit padding to 56 bytes
}

// Decode parses one ring-buffer sample.
func Decode(b []byte) (Event, error) {
	var ev Event
	if len(b) < Size {
		return ev, fmt.Errorf("short ring buffer sample: %d bytes, want %d", len(b), Size)
	}
	if err := binary.Read(bytes.NewReader(b[:Size]), binary.LittleEndian, &ev); err != nil {
		return ev, err
	}
	return ev, nil
}
