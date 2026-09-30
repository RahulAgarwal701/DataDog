# Mini Datadog - Phase 1 demo runbook

One command starts everything. Start it **at least 10 minutes before you present** so the
graphs have history.

```bash
make demo          # build + start the stack, wait until every container is healthy
make -C demo ps    # container status
make -C demo logs  # tail everything
make -C demo down  # stop (add `nuke` to also wipe the log volume)
```

Then open **<http://localhost:3000>**.

| URL | What it is |
|---|---|
| <http://localhost:3000> | dashboard (the only thing you need open) |
| <http://localhost:9000/api/v1/health/all> | Platform API, the single entry point the UI uses |
| <http://localhost:9090> | Prometheus (evidence: 4 targets UP) |
| <http://localhost:9003/api/v1/topology> | Topology API, raw JSON |

`make demo` ends by printing a pass/fail line per backend. If any line says `FAIL`, run
`make -C demo logs` and fix that before you present.

---

## The 8-minute script

| # | Do this | What the supervisor sees |
|---|---|---|
| 1 | Architecture slide, **LIVE** vs **PLANNED** | Honest scope: anomaly detection and correlation are Phase 2 |
| 2 | **Overview** tab | 4 services healthy, live req/s, p95, error % |
| 3 | **Metrics** tab, switch service and time range | Real time-series from Prometheus, updating |
| 4 | **Logs** tab: filter `payments` + `ERROR`, click a `request_id` | One request traced across services |
| 5 | **Network** tab | TCP connections resolved to `orders -> payments` with no tracing library in the services |
| 6 | `make -C demo fault-latency` | p95 jumps within ~30 s, WARN `slow request` logs, connection durations lengthen |
| 7 | `make -C demo fault-error` then `make -C demo fault-refuse` | Error-rate spike, ERROR logs, `orders->payments` goes red with `ECONNREFUSED` |
| 8 | `make -C demo fault-restore` | Metrics recover |
| 9 | `git log --oneline` | Contract-first, per-module commit history |

One command runs the whole fault sequence with pauses: `make -C demo faults`.
Reset for a clean run: `make -C demo fault-restore` (under 30 s).

---

## The eBPF tab: read this before you present

The collector is real: `linux/ebpf` is a Go agent that loads `bpf/netmon.c` on the
`sock:inet_sock_set_state` tracepoint. **It needs `CAP_BPF`, so it needs root.**

By default the Network tab is fed by `demo/netmon_sim.py`, which emits the same
contract-shaped `NetworkEvent` stream but **synthesises it rather than reading the
kernel**. It reads real state (loadgen rps, each service's `/health`, each service's
`latency_p95_ms`), so it reacts correctly to the faults, but it is not a kernel
observation.

**If the supervisor asks "is that real eBPF?", say which one is running.** Two honest
options:

**A. Real agent, on this machine (recommended, ~5 s of setup).** A password dialog will
appear - that is the OS asking to run the agent as root:

```bash
make -C demo netmon-sim-stop   # so the two sources do not double-count
make -C demo netmon-ebpf
```

Leave it running in its own terminal. The Network tab now shows real kernel events, and
`--pretty` prints them readably, which is the strongest version of demo step 5. It also
detaches the tracepoint when you stop it.

Verified working on this machine. The agent resolves real PIDs to real service names and
reports the genuine failure, for example:

```
CONNECT  orders(117770) -> payments:8002
CLOSE    orders(117770) -> payments:8002  lived 83.7ms
CONNECT_FAILED  orders(117770) -> payments:8002  ECONNREFUSED
```

**B. No root available.** Run with the simulator and say so. The topology graph, the edge
counts and the `ECONNREFUSED` behaviour are all still real data flowing through the real
Topology API.

To switch back to the simulator, or to capture real kernel events as committed evidence
for the report:

```bash
make -C demo netmon-sim-start
make -C demo ebpf-sample     # ~30 s of live traffic, then Ctrl-C
# -> linux/docs/samples/events.sample.jsonl
```

Prep without the agent: `make -C demo ebpf-build` (needs clang) and see
`linux/docs/ebpf-setup.md`.

> **Watch for orphaned agents.** If you Ctrl-C the `timeout`/`pkexec` wrapper from another
> terminal, or the terminal is killed, the agent keeps running as root and keeps posting
> events. Symptom: self-edges in the topology, or double-counted connections from two
> sources. `ps -eo pid,args | grep '[b]in/agent'`; kill leftovers with
> `pkexec kill -9 <pids>` (root-owned, so plain `pkill` will not work).

---

## If something is broken

| Symptom | Fix |
|---|---|
| Network tab empty | `make -C demo ps` - is `netmon-sim` up? `make -C demo logs netmon-sim` |
| Metrics tab says "upstream_unavailable" | Prometheus needs ~20 s on first boot. `make -C demo ps` |
| Logs tab empty | The shipper indexes on a 1 s poll. Wait ~10 s, then `make -C demo logs log-shipper` |
| Payments shows gaps in metrics | That is the connection-refusal fault still active. `make -C demo fault-restore` |
| Port already in use | `ss -ltnp \| grep -E '800[0-3]\|8010\|900[0-3]\|3000'` |
| ES slow / OOM | It is capped at 512 MB heap. Close other apps; `free -h` |
| Everything wedged after a crash | `make -C demo nuke && make demo` (wipes logs too) |

`make -C demo smoke` is the referee: it validates every Platform API response against the
pydantic contract models and prints `SMOKE GREEN` or `SMOKE RED`. Run it before you
present and again after the fault demo.

`make -C demo check` additionally runs the Go unit tests for the topology and eBPF modules.

---

## What is real and what is not (say this out loud)

**Real, end to end:** the four microservices and their call chain, Prometheus scraping
them every 5 s, the Metrics API running the contract's exact PromQL, the JSONL logs being
shipped into Elasticsearch and searchable, the Topology API deriving the service graph
from `NetworkEvent`s, the Platform API proxying all of it, fault injection through
`PUT /admin/fault`.

**Synthetic, and labelled as such:** the source of `NetworkEvent`s when
`netmon-sim` is running instead of the eBPF agent.

**Phase 2, not built:** anomaly detection (`:9004`), correlation (`:9005`), incidents.
The dashboard has an **"Incidents (Phase 2, planned)"** tab that is deliberately disabled
so the scope is visible rather than hidden.
