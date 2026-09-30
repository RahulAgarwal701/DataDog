# Brajesh — eBPF collector + Topology API

**Folders you own:** `linux/ebpf/`, `linux/topology/`
**Ports:** 9003 (Topology API), plus the agent runs on the host
**Phase 2/3:** edge persistence, agent overhead measurement, registry hardening, K8s DaemonSet

## What you built, in one paragraph

A BPF program attached to the kernel's TCP state-transition tracepoints. It emits three
kinds of event — connect, close, connect-failed — keyed on the socket pointer so a close
can be matched back to the open that created it. A Go agent reads those events off a ring
buffer, resolves the PID to a service name by reading `/proc/<pid>/environ`, converts them
to the contract's `NetworkEvent` shape, and POSTs them to the Topology API. The Topology
API keeps a bounded in-memory ring of events and derives the service graph over a sliding
window.

The headline: **no service was instrumented for this.** The services know nothing about
the collector. That is the part worth saying out loud.

## The two deliverables

**`linux/ebpf/` — the collector**

| Piece | What it is |
|---|---|
| `bpf/netmon.c` | the BPF program; socket-state tracepoints |
| `internal/loader/` | generated bindings (`netmon_bpfel.go`) + the embedded `.o` |
| `internal/convert/` | raw event → contract `NetworkEvent` |
| `internal/resolve/` | PID → service name, port → service name |
| `internal/sink/` | batched POST to the Topology API |
| `cmd/agent/` | the CLI |

**`linux/topology/` — the store and API**

| Route | Purpose |
|---|---|
| `GET /health` | liveness |
| `POST /api/v1/network-events` | ingest (the agent posts here) |
| `GET /api/v1/network-events` | read back, filterable by type |
| `GET /api/v1/topology` | the derived graph |

`GET /api/v1/topology?window_seconds=N` returns nodes and edges. `loadgen` comes back as
`kind: "external"` because that is what the registry says it is; the four services are
`kind: "service"`.

Note the edge field names are `source` / `target` (not `src_service` / `dst_service` — the
`NetworkEvent` uses those, the derived graph does not), plus `connection_count`,
`failed_count`, `first_seen`, `last_seen`, `status`. Captured live during a refusal fault:

```json
{
  "source": "orders", "target": "payments",
  "first_seen": "...", "last_seen": "...",
  "connection_count": 258, "failed_count": 10, "status": "ACTIVE"
}
```

## How to demo it

```bash
make -C demo netmon-sim-stop    # stop the simulator, it is not you
make -C demo netmon-ebpf        # a password dialog appears; that is the OS, not a bug
```

Leave it running in its own terminal. `--pretty` prints one readable line per event:

```
CONNECT        orders(117770) → payments:8002
CLOSE          orders(117770) → payments:8002  lived 83.7ms
CONNECT_FAILED orders(117770) → payments:8002  ECONNREFUSED
```

Then in the browser, Network tab: four edges, real connection counts, and on
`make -C demo fault-refuse` the `orders → payments` edge turns red.

**If you do not run the agent**, the Network tab is fed by `demo/netmon_sim.py`. Say so
plainly: "this is the simulator, it reads the same real service state but synthesises the
events rather than reading the kernel." A supervisor will ask, and the honest answer is
worth more than the bluff.

To switch back: `make -C demo netmon-sim-start`.

## Questions you will actually get

**"Is that really eBPF, or is it reading logs?"**

Genuinely eBPF. Show the PIDs: the agent prints `orders(117770)`. That number is the real
PID of the orders uvicorn process, resolved by reading `/proc/117770/environ` for
`SERVICE_NAME`. The alternative design — instrumenting each service to report its own
calls — is what the agent exists to avoid.

Evidence is committed: `linux/docs/samples/events.sample.jsonl` is 2298 events captured
from the running stack with a refusal fault injected partway through (1108 connects, 1108
closes, 82 `ECONNREFUSED`). It validates against the contract models with zero violations.

**"How do you tell a refused connection from a timeout?"**

This is the honest weak point, so lead with it. The tracepoint does not expose `sk_err`,
so the agent infers it from **how quickly the failure happened**: a refused connection
gets an RST in well under a second, a timeout takes seconds of SYN retransmits. The
threshold is one second. It is a heuristic, it is in `convert.go` under a comment saying
so, and it is unit-tested at the boundary.

If pressed on how you would fix it properly: `bpf_probe_data` on `sk_err`, or read
`sk->sk_err` from a `kprobe` on `tcp_close`. More fragile, more accurate.

**"Why do you drop loopback?"**

A service's healthcheck dials its own port, and the probe process inherits the
container's `SERVICE_NAME`. Without a filter, every node reports a connection *to itself*
every few seconds and the graph fills with self-loops. The topology is about how services
talk to each other, so loopback is dropped. `--emit-unresolved` keeps it, so you can
prove the filter is what removed it. There is a test for exactly that
(`TestDropsLoopback`).

**"Why does it need root?"**

Loading a BPF program needs `CAP_BPF` plus a raised memlock limit, and the agent has to
read `/proc` for every process on the box. There is no unprivileged path short of a
privileged helper, which would be a bigger answer than this phase asks for. `pkexec` is
how the demo gets it. A K8s DaemonSet with a `hostPID` + `privileged` security context is
the production shape, and it is Phase 3.

**"How much overhead does it add?"**

Not measured, and it is on the Phase 3 list. Say that. What you can say: the agent batches
up to 200 events per POST on a 1s flush interval, and at the demo's 5 rps across 4 hops it
produces roughly 20 events/second. The BPF side only fires on socket state transitions, not
per packet, so the kernel cost is per-connection rather than per-byte.

**"What is the topology graph status logic?"**

An edge is `ACTIVE` if it was seen within the last 60 seconds, `STALE` after that
(`ActiveWithin` in `internal/store/store.go`). This is edge aging — the connection has not
necessarily died, we have just not seen it recently.

Note for honesty: edge `status` and the dashboard's colour are different things. The
dashboard colours an edge red on `failed_count`, not on `status`. So a refused edge stays
`ACTIVE` (connections *are* still being attempted) while rendering red. Both are correct;
they answer different questions. The live record above shows exactly this state:
`"failed_count": 10, "status": "ACTIVE"`. If a supervisor assumes `status` should go
`DOWN`, that is the misunderstanding to head off.

**"The red edge stays red for a while after I clear the fault — bug?"**

No. The topology counts failures over a 60s window, so the edge goes grey between 30 and
60 seconds after the fault is cleared. Measured, not guessed. If it is still red at 60s,
that is a real bug.

## Things that will trip you up

- **Orphaned agents.** Kill the `timeout`/`pkexec` wrapper and the root child keeps
  running, still posting events, and inflates the edge counts. `make -C demo ebpf-agents`
  lists them. They are root-owned so `pkill` will not work — use `pkexec kill -9 <pid>`.
  This bit me repeatedly; check it before you present.
- **One network source at a time.** The simulator and the agent both post events for the
  same traffic. `netmon-ebpf` stops the simulator first for this reason.
- **The Topology API buffer is in-memory.** Restart it and the graph empties. Useful when
  you want a clean graph; surprising if you did not expect it.
- **`HTTP_KEEPALIVE` is `false` in compose on purpose.** With keep-alive the caller reuses
  one connection and you see a single CONNECT for the whole run instead of one per
  request. The collector is counting connections, so it has to be told to make them.

## Your own commands

```bash
cd linux/ebpf && make test          # 5 packages
cd linux/topology && make test      # 4 packages
make -C demo ebpf-build            # needs clang, regenerates the bindings
make -C demo ebpf-sample           # ~30s of real events -> linux/docs/samples/
cd linux/docs && cat ebpf-setup.md # capabilities, troubleshooting, acceptance checklist
```

`linux/docs/ebpf-setup.md` has the full acceptance checklist, all seven items verified on
the demo machine.
