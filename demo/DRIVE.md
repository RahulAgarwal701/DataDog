# Driving the Phase 1 demo

> **Before you present:** read your own Q&A guide in [`docs/members/`](../docs/members/README.md).
> Those have the "why" behind every step here, the questions a supervisor is likely to ask, and
> the limitations you should volunteer yourself. This file is the click-path; those are the answers.

One page. Follow it top to bottom. Every step is one command, what you say, and what
must be true before you move on. Nothing here needs judgement from you.

Print this. Keep it next to the laptop, not in the browser.

```
Terminal A  ->  the commands below
Terminal B  ->  make -C demo logs platform-api   (only if something looks wrong)
Browser     ->  http://localhost:3000
```

---

## Before the supervisor arrives (do this 10+ min early)

```bash
make demo            # first time, or after a reboot
make -C demo preflight
```

`preflight` is the whole go/no-go in one command. It finishes in about 2 seconds and
ends with `READY`. If it does not say `READY`, do not start presenting:

| preflight says | do this |
|---|---|
| `only N/14 containers up` | `make demo` |
| `FAIL <name> (port)` | `make -C demo logs`, then `make demo` |
| `FAIL full chain returned 502` | `make -C demo reset` |
| `WARN no network events yet` | wait 10s, re-run. Or `make -C demo netmon-ebpf` (asks for your password) |
| `FAIL a fault is active` | `make -C demo reset` |
| an agent PID listed | `pkexec kill -9 <pid>` — an orphan double-counts traffic |

If you want graphs with history, leave it running 10 minutes before you start. Go and
eat. Do not present step 1 cold.

---

## The demo

Say the timings in brackets. Every one of them is measured on this machine, not guessed.

### 1. Scope — 30s, no commands

"Live is metrics, logs, network topology, and a fault-injection story. Anomaly detection
and cross-signal correlation are Phase 2. Everything you are about to see is real
traffic through four services, with no tracing library in any of them."

### 2. Overview tab — point at it, do not click yet

You should see 4 services healthy, ~5 req/s, p95 around 100ms, 0% errors.

> "Five requests a second through gateway to orders to payments to inventory."

**Must be true:** four green cards. If one is amber, the walkthrough would have failed in
preflight, so stop and `make -C demo reset`.

### 3. Metrics tab — switch service, then widen the range

Click through orders then payments, and drag the range to 15m.

> "Real time-series from Prometheus, not a fixture. p95 comes from a histogram quantile."

**Wait ~20s** for a poll cycle. **Must be true:** the line moves and is not flat.

### 4. Logs tab — this is the one that lands

Type `payments` in the service box, set severity `ERROR`. Then clear the filters and
click a `request_id` in the table.

> "One request ID, the same value in four services' logs. That is the trace — we did not
> add OpenTelemetry to make it work, the ID is just propagated."

The same `request_id` really does appear in api-gateway, orders, payments and inventory
for the same request. Verified.

**Must be true:** rows with a clickable request ID, and clicking one narrows the table to
that request's lines across the services.

**Click a row that is currently on screen.** The trace filter pulls the most recent 1000
lines and filters client-side, which is about 45 seconds of traffic at 5 rps. Scroll back
to a request from several minutes ago and the box will say "No logs match" — that is the
window, not a broken filter.

If the Logs tab is empty on arrival, the shipper indexes on a 1s poll: wait 10s, refresh.

### 5. Network tab — say which source this is

> "TCP connections, resolved from PID to service name, with no instrumentation in the
> services. This is the eBPF collector reading the kernel."

**If you ran `netmon-ebpf`:** you are showing real kernel events. Strongest version.
**If you did not:** say "this is the simulator — it reads the same real service state, but
synthesises the events rather than reading the kernel." Do not blur this. A supervisor will
ask.

Four edges: `loadgen → api-gateway → orders → payments → inventory`.

### 6. Latency fault — 15s to visible

```bash
make -C demo fault-latency
```

> "800ms injected into payments. Nothing was restarted."

**Wait 15s.** Then back to the **Metrics** tab. p95 goes `~100ms → ~975ms`, and it
propagates to orders. The **Logs** tab fills with WARN `slow request`.

**Recovery:** the fault expires by itself at 60s. The *graph* takes another ~30s to
settle, because Prometheus is on a 30s rolling window — so p95 is still high at 65s and
back to ~100ms by about 90s. Say "it clears itself" and let it, or `make -C demo
fault-restore`.

### 7. Error fault — 30s to visible

```bash
make -C demo fault-error
```

> "50% of payments requests now fail. Watch it propagate up the chain."

**Wait 30s** (it reads better than 15s here). **Overview** shows orders ~40-45% and
api-gateway ~45% errors. **Logs** fills with ERROR. The cascade is the point: payments
500 → orders `http_500` → gateway `http_502`.

**Recovery:** expires by itself at 60s, graph settles ~30s later. Or `make -C demo
fault-restore`.

### 8. Connection refusal — the strongest moment

```bash
make -C demo fault-refuse
```

> "This is the one worth arguing about. I am not stopping the payments *container* — I
> killed only its listening socket. The container stays up, so Docker keeps its DNS
> record and its IP. If I had stopped the container, orders would fail in getaddrinfo
> and never issue a connect() — there would be no TCP packet for eBPF to see, and the log
> would say 'name or service not known' instead of 'connection refused'."

**Within 10s:**

- **Network** tab: the `orders → payments` edge turns **red**, labelled with a ✗ count
- **Logs** tab, orders: `ConnectionRefusedError: [Errno 111] Connect call failed`

**Do not type the error string into the Logs search box.** The search only matches the
`message` field, and this text lives in `fields.error` — searching `ECONNREFUSED` returns
"no logs match" and looks like the evidence is missing. Filter by service `orders` +
severity `ERROR` and read the rendered row instead. (Known gap, listed in
`docs/members/diya.md`.)

**Do not point at the error-rate number here.** It falls rather than rises, because
Prometheus is on a 30s rolling window that still holds the 500s from step 7. Red edge plus
the Errno 111 log line is the evidence; the percentage would undercut you.

**Recovery:** `make -C demo fault-restore`.

### 9. Recovery

```bash
make -C demo fault-restore
```

> "Same switch, back off. The faults are reversible — this is not damage."

**Wait 60s** — not 30. The topology counts failures over a 60s window, so the red edge
stays red for somewhere between 30 and 60 seconds after you restore, then goes grey on
its own. Overview is back to 0% errors within ~30s, faster than the edge. If the edge is
still red at 60s, that is a bug, not lag: `make -C demo reset`.

Showing the recovery matters as much as showing the break.

### 10. Close

```bash
git log --oneline
```

> "The contract was the first commit, then one commit per module, then the demo glue, then the
> evidence and the driving script."

Do **not** quote a commit count from memory. Count is not a virtue here, and the history grows
— the argument is the *order*: `02_CONTRACTS.md` and the pydantic models landed before any
service existed, so nothing could drift from the contract. If they ask how you proved it:

```bash
git log --oneline --reverse | grep -n . | sed -n '6,8p'   # contract sits at position 3
```

---

## If it goes wrong mid-demo

Run this the moment anything looks off. It is safe and takes about 5 seconds.

```bash
make -C demo reset
```

Clears faults, wipes the topology buffer so the graph is clean, and confirms the chain
returns 201. Then go back to step 2.

| symptom | command |
|---|---|
| a panel empty / stale | `make -C demo reset` |
| chain 502, all containers "healthy" | `make -C demo reset`. Still broken: `docker compose -f demo/docker-compose.yml -f demo/docker-compose.override.yml exec payments /supervisor.sh logs` |
| edge still red 60s after a fault cleared | `make -C demo reset` |
| edge counts look doubled | `make -C demo ebpf-agents`; `pkexec kill -9 <pid>` |
| want a clean slate entirely | `make -C demo nuke && make demo` — this wipes logs and takes ~3 min, so do it before, not during |
| total death | `make -C demo demo` |

## The one thing that cannot be fixed live

If Docker itself is wedged, the fallback is the raw APIs on the Platform API. No dashboard,
but the data is all there:

```bash
curl -s localhost:9000/api/v1/topology | python3 -m json.tool | head -40
curl -s 'localhost:9000/api/v1/logs/search?severity=ERROR&limit=5' | python3 -m json.tool
curl -s localhost:9000/api/v1/metrics/latest | python3 -m json.tool
```

## Optional, if there is time

```bash
make -C demo load-surge     # 5 -> 20 rps, watch the graphs climb
make -C demo load-baseline  # back to 5
```

Or the honest answer if asked about eBPF scope: the collector traces TCP connect/close
and classifies refused-vs-timeout by how fast the failure happened, because the tracepoint
does not expose `sk_err`. That is a heuristic, and it is in `convert.go` next to that
caveat.
