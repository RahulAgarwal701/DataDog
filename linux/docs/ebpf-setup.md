# eBPF agent + Topology API: setup, run, demo (Brajesh, Phase 1)

Go stack: **cilium/ebpf** (eBPF loader, `bpf2go`) for the agent, **stdlib `net/http`** for the Topology API.
Folders: `ebpf/` (agent + simulator) and `topology/` (API + shared contract package). Nothing outside them was touched.

## 1. Prerequisites (Arch Linux)

```bash
sudo pacman -S --needed go clang llvm libbpf make
# optional, for the real stack:  sudo pacman -S docker docker-compose
```

Sanity checks (all should succeed on the stock Arch kernel):

```bash
uname -r                                                        # >= 5.8 needed for the BPF ring buffer
sudo cat /sys/kernel/tracing/events/sock/inet_sock_set_state/format | head -25
```

The `format` output must list `skaddr` (offset 8), `oldstate` (16), `newstate` (20), `sport` (24), `dport` (26),
`family` (28), `protocol` (30), `saddr[4]` (32), `daddr[4]` (36). `bpf/netmon.c` hard-codes that layout so it
needs no `vmlinux.h`/BTF. If your kernel prints something different, tell me and I'll adjust the struct.

## 2. Build

```bash
cd topology && make setup && make test          # `make setup` writes go.sum (needs network once)
cd ../ebpf  && make setup && make generate && make test && make build
```

* `make generate` compiles `bpf/netmon.c` with clang into `internal/loader/netmon_bpfel.{go,o}`. **Commit those two files**
  so teammates and Docker builds don't need clang.
* `make test` runs the pure-Go unit tests (no root, no kernel needed).
* `contracts/registry.yaml` (Darsan's) must exist at `../contracts/registry.yaml` relative to `ebpf/` and `topology/`
  (override with `REGISTRY_PATH`). Until it exists, copy `topology/testdata/registry.yaml` there.

## 3. Run it standalone (no other team's code needed)

Terminal 1: Topology API. Add `MOCK=true` only if you want synthetic data instead of the real agent.
```bash
cd topology && make run                         # :9003
```
Terminal 2: fake microservice chain + load (from `cmd/simservice`):
```bash
cd ebpf && make demo-start                      # api-gateway:8000 -> orders:8001 -> payments:8002 -> inventory:8003, loadgen 5 rps
```
Terminal 3: the agent (root required):
```bash
cd ebpf && make run                             # = sudo ./bin/agent --pretty
```
Expected output:
```
09:30:01.412  CONNECT        loadgen(4101) → api-gateway:8000
09:30:01.414  CONNECT        api-gateway(4098) → orders:8001
09:30:01.420  CONNECT        orders(4099) → payments:8002
09:30:01.471  CONNECT        payments(4097) → inventory:8003
09:30:01.490  CLOSE          payments(4097) → inventory:8003  lived 18.7ms
```
Verify the API:
```bash
curl -s localhost:9003/api/v1/topology | jq          # 4 service nodes + loadgen(external), 4 edges
curl -s 'localhost:9003/api/v1/network-events?service=payments&limit=5' | jq
```
Fault demos (these are demo script steps 6 and 7):
```bash
make demo-slow        # payments +800 ms: CLOSE "lived" times jump to ~800 ms
make demo-refuse      # payments stopped: red CONNECT_FAILED ECONNREFUSED on orders → payments
make demo-restore
```

## 4. Run it against the real stack (Dev's services + Compose)

* The agent runs **on the host** (needs root and the host kernel), *not* inside Compose:
  `sudo ./ebpf/bin/agent --pretty --registry contracts/registry.yaml --topology-url http://localhost:9003`
* The Topology API runs in Compose: `docker build -f topology/Dockerfile .` (context = repo root). Compose snippet for Darsan:
  ```yaml
  topology-api:
    build: {context: ., dockerfile: topology/Dockerfile}
    ports: ["9003:9003"]
  ```
* **Requirement on Dev's services:** every service process must have env `SERVICE_NAME=<name>` (contract section 7.1)
  and must not reuse HTTP connections (`HTTP_KEEPALIVE=false`), otherwise you'll only see the first connection.
* Compose containers share the host kernel, so the host agent sees their traffic. It reads
  `/proc/<pid>/environ` of the container's process from the host `/proc`.

## 5. How it works

| Step | Where | What |
|---|---|---|
| 1 | kernel (`bpf/netmon.c`) | tracepoint `sock:inet_sock_set_state` on every TCP state change. `→SYN_SENT` records PID+comm of the caller of `connect()`. `SYN_SENT→ESTABLISHED` = **CONNECT**. `SYN_SENT→CLOSE` = **CONNECT_FAILED**. `→CLOSE` of an established client connection = **CLOSE** (with lifetime). Events go to a ring buffer. |
| 2 | `internal/loader` | loads/attaches the program, reads 56-byte records (`internal/raw`, layout checked by a unit test). |
| 3 | `internal/resolve` + `convert` | destination = registry lookup by port; source = `SERVICE_NAME` from `/proc/<pid>/environ` (fallback: `services.<name>` in cmdline). Events are kept only if **both** ends resolve, so Prometheus/Elasticsearch traffic never pollutes the graph. Monotonic kernel time is converted to UTC wall time. |
| 4 | `internal/sink` | batches (≤200) every second, POSTs `NetworkEventBatch` to `:9003`, retries with backoff, bounded queue. Every event is also appended to `ebpf/out/events.jsonl`. |
| 5 | `topology/` | ring buffer of 100k events (idempotent by `event_id`), derives edges from `CONNECT`/`CONNECT_FAILED`; serves `/api/v1/network-events` and `/api/v1/topology`. |

No tracing library and no change to the monitored services: only an environment variable they already need.

## 6. Contract notes (so nobody is surprised)

* `connection_count` = successful `CONNECT` events **inside the requested window**; `failed_count` = `CONNECT_FAILED` events in the window.
  `first_seen`/`last_seen` are all-time. `ACTIVE` = last seen ≤ 60 s ago, else `STALE`.
* `duration_ms` is only set on `CLOSE`, `error` only on `CONNECT_FAILED`, exactly as in the contract.
* A batch with an unknown field or > 500 events is rejected with 400 `ErrorResponse`; individually invalid events are counted in `rejected`.

## 7. Known limitations (say these before the supervisor asks)

* **IPv4 TCP only.** IPv6 and UDP are not captured.
* **`ACCEPT` events are not emitted yet** (the contract allows them): server-side accept needs a second probe. Planned for Phase 2. Topology edges only need client-side events.
* **`ECONNREFUSED` vs `ETIMEDOUT` is inferred from time-to-failure** (< 1 s → refused), because this tracepoint does not expose `sk_err`. Phase 3: read `sk_err` via a `tcp_done` probe.
* A process that exits within microseconds of `connect()` cannot be resolved from `/proc`; those are counted as `unresolved_src` in the stats line, never guessed.
* Source resolution relies on `SERVICE_NAME`. In Phase 3 the registry-based IP/port mapping and container-label fallback harden this.

## 8. Troubleshooting

| Symptom | Fix |
|---|---|
| `load eBPF objects: ... operation not permitted` | run as root (`sudo`); check `dmesg | tail` |
| `attach tracepoint ... no such file` | tracefs not mounted: `sudo mount -t tracefs nodev /sys/kernel/tracing` |
| build error `netmonObjects undefined` | run `make generate` first |
| clang: `'bpf/bpf_helpers.h' file not found` | `sudo pacman -S libbpf` |
| agent runs but prints nothing | look at the stats line: `unresolved_src` high → service missing `SERVICE_NAME`; `unregistered_port` high → ports differ from `registry.yaml`; try `--emit-unresolved --pretty` |
| only the first request shows up | the caller reuses connections; disable HTTP keep-alive |
| `go mod tidy` cannot find `github.com/minidd/topology` | run it from `ebpf/` (the `replace` points at `../topology`) |

## 9. Capturing review evidence (do this on real runs; never hand-edit)

```bash
cd ebpf && make demo-start && make sample            # Ctrl-C after ~30 s → docs/samples/events.sample.jsonl
curl -s localhost:9003/api/v1/topology | jq > ../docs/samples/topology.sample.json
```
Also screenshot the `--pretty` terminal during `make demo-refuse` (red CONNECT_FAILED lines).

## 10. Phase 1 acceptance checklist

Verified on the demo machine against `demo/` (all containers healthy, `make -C demo check` green):

- [x] `make test` passes in `topology/` and `ebpf/` (4 and 5 packages)
- [x] `make run` prints CONNECT events for api-gateway→orders, orders→payments, payments→inventory
- [x] `make demo-refuse` produces CONNECT_FAILED / ECONNREFUSED for orders→payments within ~1 s
- [x] `GET /api/v1/topology` returns 4 service nodes + loadgen external and the chain edges
- [x] `docs/samples/*.sample.*` generated from a real run and committed
- [x] Verified on the demo machine (not just your laptop)
- [x] Darsan's `make smoke` passes against `:9003`

Notes from that run, in case they come up again:

The agent needs root (CAP_BPF plus a raised memlock limit). `pkexec` prompts once
and is enough; a Python rewrite would hit the same kernel restriction.

`docs/samples/events.sample.jsonl` was captured with a refusal fault injected partway
through, so it contains real `ECONNREFUSED` events rather than only healthy traffic.
It validates against the pydantic models with no violations.

Watch for orphaned agents. Killing the `timeout`/`pkexec` wrapper orphans the root
child instead of stopping it, and an orphan keeps posting events, which shows up as
inflated edge counts. `make -C demo ebpf-agents` lists them; they need
`pkexec kill -9`.
