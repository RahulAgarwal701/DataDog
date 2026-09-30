# Demo script — Phase 1

**The one file to follow while presenting.** Each step shows the command to run (as a
caption under the step) and what to say.

Related: [`DRIVE.md`](DRIVE.md) is the longer click-by-click reference with troubleshooting.
[`../docs/members/`](../docs/members/README.md) is the per-person Q&A prep.

---

# PART 1 — SETUP

**Do this 10–15 minutes before the supervisor arrives.** Not during.

### 1.1 Start everything

```bash
make demo
```

<sub>~33s. Ends with "dashboard http://localhost:3000".</sub>

### 1.2 Confirm it's safe

```bash
make -C demo preflight
```

<sub>Must end <b>READY</b>. 13 core containers, chain 201, 5.0 rps, no faults.</sub>

### 1.3 Real eBPF agent (optional but stronger)

If you want the real kernel collector instead of the simulator — **stop the simulator
first**, because both must not run at once:

```bash
make -C demo netmon-off
```

<sub>Output: "netmon-sim stopped."</sub>

```bash
sudo ./linux/ebpf/bin/agent --pretty --topology-url http://127.0.0.1:9003 --registry ./darsan/contracts/registry.yaml
```

<sub>Enter password. Leave running in a <b>second terminal</b> — closing it kills the agent.
Scrolls lines like <code>CONNECT orders(117770) → payments</code>.</sub>

Verify — both must be true:
```bash
make -C demo preflight
```
- says `13 core containers up (simulator off -- real eBPF agent owns the Network tab)`
- the orphan section shows an agent PID

Then reload the browser and confirm the Network tab reads
`network source: eBPF agent, real kernel events (host conputrr)`.

> **Skipped it?** Fine. The Network tab reads `SIMULATED` and you narrate the fallback —
> see Part 4. Everything else in the demo is identical.

### 1.4 Browser

Open <http://localhost:3000>, **Ctrl+Shift+R** (nginx caches). Leave on Overview.

**Let it sit 5+ minutes** so the graphs have history. This is the single biggest
difference between a good demo and an empty one.

### 1.5 Load generator — leave it alone

```bash
curl -s localhost:8010/profile
```

<sub>Must say <code>{"profile":"baseline","rps":5.0}</code>. Never run <code>load-stop</code> before a demo.</sub>

---

# PART 2 — THE DEMO

### Step 1 — Scope (45s, click nothing)

> "Four services in a real HTTP chain. Three independent things watch it — metrics, logs,
> and kernel-level network. One API in front.
>
> There's a load generator sending 5 requests a second through the chain continuously, so
> every graph has real data and the fault has something to damage. That's what makes this
> repeatable — I could show you this exact sequence in an empty room and it'd look
> identical.
>
> Let me break something."

### Step 2 — Overview (20s, point only, don't click)

> "Every number is live. One fault, three places at once."

<sub>4 green cards.</sub>

### Step 3 — Metrics (30s, select `payments`)

> "Real Prometheus data, scraped every 5 seconds. p95 is a histogram quantile, not an
> average."

<sub>Wait ~20s for the line to draw. It must move, not sit flat.</sub>

### Step 4 — Logs (45s)

> "One request ID appears in all four services. That's the trace — the ID is just
> propagated, no OpenTelemetry."

**Click any `request_id`** → filters to that request across all four services.

> "And this tab live-refreshes every 5 seconds, so I'll leave `orders` + `ERROR` set for
> the fault later."

### Step 5 — Network (45s)

> "TCP connections, resolved from PID to service name, with zero instrumentation in the
> services."

**Point at the source label under the graph:**

> "The dashboard labels its own source. It says SIMULATED because that's the stand-in that
> runs without root. The real eBPF agent stamps the machine hostname instead — that's the
> label you'd see with it running."

**Explain the numbers on the arrows:**

> "That's connections over the last 60 seconds. 5 requests a second through 4 hops, so
> roughly 300 per edge. When I break something it splits — successes on the left,
> failures with a cross on the right, and the arrow turns red."

<sub>4 edges. All four read the same number, because every request passes through all four hops.</sub>

### Step 6 — Latency fault (30s + 15s wait)

```bash
make -C demo fault-latency
```

> "payments gets 800 milliseconds of extra latency, for 60 seconds."

<sub>⏱️ <b>WAIT 15s.</b> p95 goes ~98ms → ~950ms. Point at the jump.</sub>

### Step 7 — Error fault (30s + 30s wait)

```bash
make -C demo fault-error
```

> "Now 50% of requests come back as a 500."

<sub>⏱️ <b>WAIT 30s.</b> Error rate climbs on Overview and Metrics.</sub>

### Step 8 — Connection refusal (45s + 10s wait) — **the strongest moment**

```bash
make -C demo fault-refuse
```

> "This is the one worth arguing about. I'm not stopping the payments *container* — I
> killed only its listening socket. Docker keeps the DNS record and the IP, so orders
> actually issues a connect() and the kernel produces a real TCP RST.
>
> If I'd stopped the container instead, orders would fail during DNS resolution. There'd
> be no TCP packet at all, the log would say 'name or service not known', and the eBPF
> collector would correctly see nothing."

<sub>⏱️ <b>WAIT 10s.</b> The orders→payments arrow turns <b>RED</b>. Switch to the Logs tab —
it's already filtered — and new lines with <code>Errno 111</code> are appearing live.</sub>

> "Same failure. Two independent observers. No correlation between them."

### Step 9 — Recovery (60s wait)

```bash
make -C demo fault-restore
```

> "Same switch, back off. These faults are reversible — this is not damage."

<sub>⏱️ <b>WAIT 60s.</b> The red arrow greys out on its own. Say: "it counts failures over a
rolling 60-second window, so it drains slowly." Measured: still red at +38s, clear at +68s.</sub>

### Step 10 — Close (30s)

```bash
git log --oneline
```

> "The contract was the first commit, then one commit per module, then the demo glue.
> Nothing was written until the interface was agreed."

```bash
git log --oneline --reverse | sed -n '7p'
```

<sub>The contract commit — 3rd in the history. That's your proof, not a claim.</sub>

---

# PART 3 — WHEN THINGS LOOK WRONG

| What you see | What you say | Fix it? |
|---|---|---|
| **Step 8: error rate goes DOWN** | "That's Prometheus's 30-second window still holding the 500s from the previous step. The red arrow and the log line are the real evidence." | **No.** Never point at the error % here |
| **Step 9: arrow still red** | "It counts failures over 60 seconds, so it drains slowly." | **No.** Wait it out |
| Arrow still red after 90s | "That's a real bug, not lag." | `make -C demo reset` |
| Logs search says "No logs match" | You typed `ECONNREFUSED`? Known gap — search only covers `message`, the error text is in `fields`. Use service + severity filters. | No |
| Edge counts look doubled | Two network sources running at once. | `make -C demo ebpf-agents` |
| An agent PID is missing from preflight | The collector died. | `make -C demo netmon-sim-start` (no password; label flips to SIMULATED) |
| Any panel empty or stale | — | `make -C demo reset` |
| Total death | — | `make demo` |

## Emergency — the one command

```bash
make -C demo reset
```

<sub>Clears all faults, wipes the graph, re-verifies the chain. ~5s. Safe mid-demo.</sub>

---

# PART 4 — Q&A

## "Is the eBPF just a simulator?"

**Lead with yes-then-here's-the-difference.** It disarms him and it's completely honest.

> "**Right now, on screen, yes** — it's the simulator. It's labelled as such on the
> dashboard deliberately, so nobody is misled.
>
> Here's the difference. The simulator is 200 lines of Python that *asks questions* — is
> payments up, what's its latency — and invents events matching the answers. The real
> collector is a BPF program **compiled into the kernel**. 9,320 bytes of compiled eBPF
> bytecode, committed to the repo, attached to a single kernel tracepoint.
>
> The simulator invents the PIDs. The real one reads them from `/proc`."

**Then show the proof — this is what wins:**

```bash
python3 -c "
import json,collections
c=collections.Counter()
for l in open('linux/docs/samples/events.sample.jsonl'):
    c[json.loads(l)['event_type']]+=1
print(sum(c.values()),'events'); print(dict(c))"
```

<sub>2298 events · 1108 CONNECT · 1108 CLOSE · 82 CONNECT_FAILED</sub>

> "2,298 events I captured off the kernel on this machine, with a refusal fault injected
> partway through — those 82 `CONNECT_FAILED` are the fault. Host `conputrr`. Five distinct
> PIDs, process name `uvicorn`. The file is committed, so it's verifiable, not a claim."

**What you built:**

> "A BPF program on `sock:inet_sock_set_state`, the kernel's socket state transition
> tracepoint. One event per connection, not per packet. Keyed on the socket pointer, so a
> close is matched back to the open that created it.
>
> A Go agent reads those off a ring buffer, resolves the PID to a service name by reading
> `/proc/<pid>/environ` for `SERVICE_NAME`, converts to the contract shape, and batches
> them to the Topology API. Five packages of tests, all passing.
>
> The point: **no service was instrumented for any of that.** The services contain zero
> code for the collector."

**If he asks why it isn't running right now:**

> "It needs `CAP_BPF` to load, so it needs root. This machine has the polkit library but
> no polkit agent, so there's no way to prompt for a password. That's an environment gap,
> not a code gap — one command with sudo and it runs. That's where the sample file came
> from."

**If he asks the weakest part — volunteer this, it buys credibility:**

> "The tracepoint doesn't expose `sk_err`, so I can't read the error code directly. I infer
> refused-vs-timeout from timing — a refusal gets an RST in well under a second, a timeout
> takes seconds of SYN retransmits. One-second threshold, it's a heuristic, and there's a
> comment in the code saying so. The proper fix is `bpf_probe_data` on `sk_err` — more
> fragile, more accurate."

## Other eBPF questions

**"How do you know it's not just reading logs?"**
> "The PIDs. 71,439 / 117,770 / 117,845 — those are real process IDs of the uvicorn
> workers, resolved by reading `/proc/<pid>/environ`. A log can't tell you that."

**"Why drop loopback?"**
> "A healthcheck dials its own port, and the probe inherits the container's `SERVICE_NAME`,
> so every node reports a connection *to itself* every few seconds and the graph fills
> with self-loops. Loopback is dropped, but `--emit-unresolved` keeps it so you can prove
> the filter is what removed it. There's a test for exactly that."

**"What about performance overhead?"**
> "Not measured, and it's on the Phase 3 list — I'll volunteer that. What I can say: it
> fires per socket state transition, not per packet, so the kernel cost is per-connection
> rather than per-byte. At 5 rps across 4 hops I measured 43 events per second against an
> expected 40."

**"What if two services share a port?"**
> "The registry maps port to service, so it's resolved at config time. Two services sharing
> a port is a config error the registry catches."

## Architecture questions

**"Walk me through the architecture."**
> Four FastAPI services in a chain. Three data planes read them — Prometheus scrapes
> `/metrics` every 5s, a shipper tails JSONL into Elasticsearch, eBPF posts kernel TCP
> events. Platform API on :9000 fronts all three. Dashboard on :3000 is one static HTML
> file with no build step, talking only to :9000.

**"Is this real data or fixtures?"**
> Real. Metrics from live Prometheus scrapes, logs from real requests, network from the
> kernel or a simulator that's labelled. There's a `?mock=1` mode that runs the whole UI
> on fixtures with no backend — useful for UI work, not for this.

**"How do you know the components fit together?"**
> The contract, and `make -C demo smoke`, which parses every API response into the
> contract's pydantic models. If anyone drifts, the test fails rather than a panel going
> blank during the demo. The contract was the first commit in the history — that's the
> proof.

**"What's not finished?"**
> Anomaly detection and the correlation engine — an anomaly firing, fetching nearby logs
> and network edges, and emitting one scored incident. That's Phase 2, and there's a
> visibly disabled **Incidents** button on the dashboard marking the boundary.

**"Why one machine?"**
> eBPF needs a real Linux kernel, so it can't be hosted containers. One Linux VM, Docker
> Compose, 14 services.

---

# QUICK REFERENCE

```bash
make demo                     # start everything (~33s)
make -C demo preflight        # go/no-go, must say READY
make -C demo reset            # clear faults + wipe graph (~5s)

make -C demo fault-latency    # payments +800ms for 60s   -> visible in 15s
make -C demo fault-error      # payments 50% 500s for 60s  -> visible in 30s
make -C demo fault-refuse     # kill listener, real RST   -> visible in 10s
make -C demo fault-restore    # undo it                  -> clear in 60s

make -C demo walkthrough      # prove the chain end-to-end
make -C demo smoke            # validate every response against the contract
make -C demo load-baseline    # back to 5 rps
make -C demo load-surge       # 20 rps
make -C demo ebpf-agents      # list collector processes
```

| Timing | |
|---|---|
| latency fault visible | ~15s |
| error fault visible | ~30s |
| refusal → red arrow | ~10s |
| restore → arrow clears | ~60s |