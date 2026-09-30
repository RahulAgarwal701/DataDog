//go:build tools

// Package tools pins the bpf2go code generator in go.mod / go.sum.
package tools

import _ "github.com/cilium/ebpf/cmd/bpf2go"
