# Dev — four microservices, load generator, fault injection

**Folder you own:** `mini-datadog-dev-phase1/mini-datadog/`
**Ports:** 8000 api-gateway, 8001 orders, 8002 payments, 8003 inventory, 8010 loadgen
**Phase 2/3:** anomaly detection, retries, API latency benchmarks, per-service resource
overhead, config management

## What you built, in one paragraph

Four real services in a real HTTP chain, plus a load generator that drives traffic through
it, plus a fault-injection endpoint on every service. All four share one scaffold
(`services/common/app.py`) so they behave identically where it matters — same log format,
same Prometheus metrics, same fault semantics — and differ only in the business logic
inside them. The observability is not bolted on afterwards; it is in the scaffold, so
there is no service that can forget to emit it.

## The chain

```
loadgen :8010 ──▶ api-gateway :8000 ──▶ orders :8001 ──▶ payments :8002 ──▶ inventory :8003
```

`api-gateway` does no business logic; it validates and forwards. The registry
(`darsan/contracts/registry.yaml`) is what defines this chain and the dependencies, and
`api_gateway` reads it rather than hard-coding the order.

## The three deliverables

**`services/common/app.py`** — the scaffold. Every service gets from it:

- JSONL logging to the shared volume, one `LogRecord` per line, contract-validated
- a propagated `X-Request-Id` across every hop
- Prometheus counters and histograms on `/metrics`
- `GET /health`, `GET /admin/fault`, `PUT /admin/fault`
- CORS
- a global exception handler that logs the **root cause**, not just the message

**`services/{api_gateway,orders,payments,inventory}/`** — the four services

**`services/loadgen/main.py`** — the traffic source. `POST /profile` with
`baseline` (5 rps), `surge` (20 rps) or `stop`. `AUTOSTART=true` in compose, so traffic
starts with the stack.

## Fault injection

```bash
curl -X PUT localhost:8002/admin/fault -H 'Content-Type: application/json' \
  -d '{"type":"latency","latency_ms":800,"duration_seconds":60}'
```

`FaultConfig` is one pydantic model, so all four services take the same three shapes:

| Field | Meaning |
|---|---|
| `type` | `none` \| `latency` \| `error` |
| `latency_ms` | `await asyncio.sleep()` before responding |
| `error_rate` | probability in `[0,1]` of returning a 500 |
| `duration_seconds` | auto-expiry; `None` means until cleared |

Two details worth defending:

**The fault sleeps in the request path, not in the metrics path.** `asyncio.sleep` keeps
the event loop free, so an 800ms sleep on payments does not stop payments from serving other
requests. The p95 rises because requests genuinely take longer, not because the service
locked up. The error-rate fault uses `random.random() < error_rate` per request, so the
observed rate is statistical, not deterministic — which is the point, a real 5xx storm is
partial. That is also why the demo fault is set to `error_rate: 0.5` and not `1.0`: a 50%
fault produces a partial storm, and a partial storm is the more realistic and more
interesting failure mode.

**`duration_seconds` auto-expires.** A fault that only clears when you remember to clear
it will still be active when the supervisor asks an unrelated question. Every demo fault
has a 60s ceiling, and `make -C demo reset` clears them in one call. The ceiling has to be
longer than the 30s Prometheus rate window, otherwise the metric recovers before the fault
does and you cannot show the recovery.

## How to demo it

```bash
make -C demo fault-latency    # payments +800 ms for 60 s; p95 climbs in ~15 s
make -C demo fault-error      # payments 50% 5xx for 60 s; error rate spikes in ~30 s
make -C demo fault-refuse     # kill payments' listener -> real ECONNREFUSED
make -C demo reset            # all cleared, ~5 s
```

Live proof of the chain, no dashboard needed:

```bash
make -C demo walkthrough
```

prints the full POST /orders result and fails if the status is not 201 with
`"status":"confirmed"`.

## Questions you will actually get

**"Are these real services or stubs?"**

Real HTTP services. Every request in the chain is a separate `httpx` call over a TCP
socket, which is what makes the eBPF collector work — there are real connections to see.
The business logic is deliberately shallow (validate, forward, return a synthetic order
id) because the interesting part of this project is the observability, not commerce.

**"Why a shared scaffold instead of four independent services?"**

Because four independent implementations of logging, metrics, and fault injection drift.
They end up with different log field names, different metric labels, different fault
semantics, and then the metrics API cannot aggregate them because the label sets do not
match. One scaffold makes the four services *legibly identical* from the outside, which is
exactly what the aggregation layer needs. The cost is that the services are coupled to each
other through that file; with genuinely different languages or frameworks they would need
the contract enforced by tests instead.

**"Why does `api-gateway` exist at all?"**

It is the entry point and it validates the request before it enters the chain, which means
a bad request never produces four log lines and two failed eBPF connections. In a real
system it would also be where auth, rate limiting, and routing live. It is here mainly to
give the topology a realistic four-hop shape and a clear owner.

**"How do I know the fault is working?"**

Three independent signals, and you should point at all three. Note that the log line differs
by fault type, which is itself worth knowing:

- **latency / error faults** — the service logs `injected fault triggered` (with
  `fault_type=latency` or `fault_type=error`), because the fault code is what emitted it.
- **refusal fault** — orders logs `downstream call failed`, because orders did not inject
  anything; it just failed to reach payments. The proof is in `fields.error`.

So: the Metrics tab p95 for latency, the `injected fault triggered` line for the fault you
injected, and for the refusal, the `Errno 111` chain in `fields.error` plus the red
`orders → payments` edge in the Network tab. One fault, three data planes, no correlation.
That agreement is the strongest thing in the demo.

Careful with the error-rate panel during the *refusal* step specifically — the rate can
appear to fall, because Prometheus is on a 30s window still holding the 5xx from a
preceding error fault. Use the red edge and the log line for that step. Rahul's doc
explains the window mechanics.

**"How much did fault injection change the service code?"**

A middleware check on an `app.state.fault` object, plus the sleep and the 500 branch. The
business logic does not know faults exist. There is a genuine argument against inlined
fault injection — a production service would not have it — and the answer is that this
whole system is a fault-injection demo platform, so it is a feature of the service, not a
contamination of it.

## Things that will trip you up

**"Why does the refusal fault need a supervisor script?"**

The interesting one, so be ready for it. The obvious way to make `orders` fail is
`docker compose stop payments` — and it produces **no** `ECONNREFUSED` from orders at all,
for two reasons. First, the container name stops resolving, so the failure happens at DNS
resolution, not at `connect()`: orders logs `Name or service not known` and the kernel
emits no connect event for eBPF to see. Second, even ignoring DNS, SIGTERM is a graceful
uvicorn shutdown, so the listening socket may still be draining rather than refusing.

So `demo/payments_supervisor.sh` is a small in-container supervisor: it owns the listener
process on its own, separate from the container, and the fault does a real process kill
(SIGTERM, escalating to SIGKILL), then probes `127.0.0.1:8002` until the port is *confirmed
refusing* before returning. That is why `make -C demo fault-refuse` takes a few seconds
instead of being instant — the probe is what makes it a real refusal rather than a racing
one. If the port is still accepting after the kill, the script exits non-zero rather than
reporting a success it cannot back up.

There is a `fault-refuse-stop` target that does the naive `docker stop`, kept specifically
to demonstrate the difference. Its own comment says to use `fault-refuse` for the faithful
demo.

Worth volunteering: this is the single most interesting engineering problem in the project,
and it is completely invisible unless you know to look for it. The naive version looks like
it works — you still get an error in the dashboard — it is just measuring the wrong thing.

**The `Errno 111` log line.** When payments is killed, orders logs a failure. The scaffold
walks both `__cause__` and `__context__` to find the root cause, so `fields.error` contains
the whole chain rather than just the outer wrapper:

```
ConnectError: All connection attempts failed -> ConnectionRefusedError: [Errno 111] Connect call failed ('172.19.0.7', 8002)
```

The `->` is the exception chain. Without the walk, `str(e)` on the httpx `ConnectError`
would only ever produce "All connection attempts failed" — no `Errno 111`, no proof that
this was a *refusal* rather than a timeout or a DNS failure. That distinction is the entire
point of your supervisor script, so make sure the log evidence and the Network tab's red
edge are shown as agreeing.

Note the detail lives in `fields.error`, not in `message` — `message` stays the generic
"downstream call failed". See Diya's doc for why that matters for search.

**`duration_seconds` and the 30s window.** Faults are set to 60s because Prometheus is on
a 30s `rate()` window. When the fault expires, the window still holds the bad samples, so
the graph stays elevated for roughly another 30s after the fault is gone. Do not clear early
and then claim the fix did not work — that 30s tail is expected, and it is a good thing to
point out because it shows you understand your own measurement window.

**No unit tests in your folder.** This is a real gap and worth naming yourself. The
services are covered end-to-end by `make -C demo smoke` and `make -C demo check`, which
exercise the whole chain and validate every response against the contract, but there is no
per-service `pytest` suite the way Rahul's and Diya's components have. It is the obvious
next piece of work.

**Do not use `curl localhost:8010/orders` to test the chain.** The loadgen only exposes
`/health`, `/metrics`, `/profile` and the loadgen's own posts. Go through port 8000.

## Your own commands

```bash
make -C demo check                # Go tests + smoke + walkthrough, your chain end to end
make -C demo smoke                # contract validation across all four services
curl localhost:8000/health
curl -X POST localhost:8010/profile -d '{"profile":"surge"}' -H 'Content-Type: application/json'
curl localhost:8010/profile
make -C demo reset                # clear every fault on every service
```

Run locally without Docker with `run_local.sh` in the services folder.
