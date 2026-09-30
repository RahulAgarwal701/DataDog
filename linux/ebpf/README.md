# ebpf/: eBPF network agent (Go + cilium/ebpf)

Observes TCP connections on the host with one tracepoint, resolves them to service-to-service events and ships
them to the Topology API. Full guide: [`../docs/ebpf-setup.md`](../docs/ebpf-setup.md).

```bash
make setup generate test build     # once
make demo-start                    # simulated services (or use Dev's real stack)
make run                           # sudo ./bin/agent --pretty
```

| Path | Purpose |
|---|---|
| `bpf/netmon.c` | the eBPF program (`sock:inet_sock_set_state`) |
| `internal/loader` | loads/attaches it, reads the ring buffer (Linux only, uses generated `netmon_bpfel.go`) |
| `internal/raw` | 56-byte kernel record layout (must match `struct event`) |
| `internal/resolve`, `internal/convert` | port/PID → service names, raw → contract `NetworkEvent` |
| `internal/sink` | batching + retry POST to `:9003` |
| `internal/pretty` | readable terminal output |
| `cmd/agent` | the agent binary (`--help` for flags) |
| `cmd/simservice`, `scripts/demo.sh` | stand-in microservice chain for development and demos |

Flags worth knowing: `--pretty`, `--json`, `--dry-run`, `--out-file`, `--emit-unresolved`, `--topology-url`, `--registry`.
