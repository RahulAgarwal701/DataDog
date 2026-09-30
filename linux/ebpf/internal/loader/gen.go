package loader

// Compiles bpf/netmon.c and generates netmon_bpfel.go + netmon_bpfel.o in this directory.
// Needs: clang, llvm, libbpf headers (see docs/ebpf-setup.md). Run with `make generate`.
//go:generate go run github.com/cilium/ebpf/cmd/bpf2go -cc clang -cflags "-O2 -g -Wall" -target bpfel netmon ../../bpf/netmon.c
