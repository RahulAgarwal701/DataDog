# topology/: Topology API (Go, stdlib only) : port 9003

Ingests `NetworkEvent`s from the eBPF agent and serves them plus the derived service dependency graph.
Implements contract section 7.4 exactly. `pkg/contract` is a Go mirror of the pydantic DTOs and is also imported by `../ebpf`.

```bash
make setup test
make run          # real mode (needs the agent)
make run-mock     # MOCK=true: synthetic live events, no Linux/eBPF required
```

| Endpoint | Notes |
|---|---|
| `GET /health` | `HealthResponse` |
| `POST /api/v1/network-events` | `NetworkEventBatch` (≤ 500) → 202 `IngestAck`; unknown fields → 400; idempotent per `event_id` |
| `GET /api/v1/network-events` | `service`, `event_type`, `start`, `end`, `limit` (≤1000), `offset`; newest first; `Page[NetworkEvent]` |
| `GET /api/v1/topology?window_seconds=300` | `TopologyGraph` (1..86400 s) |

Env: `PORT` (9003), `REGISTRY_PATH`, `MAX_EVENTS` (100000), `MOCK`, `LOG_LEVEL`. Errors are always `ErrorResponse`.
Docker: `docker build -f topology/Dockerfile .` from the repo root; the binary supports `-healthcheck` for the container probe.
