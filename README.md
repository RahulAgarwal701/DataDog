# Mini Datadog — prototype

A scaled-down 3-pillar observability stack (metrics + logs + traces) with a
correlation/anomaly layer on top, exactly like the shape you sketched. No
database — every service is a single Flask process holding everything in
memory. It's meant to be read end-to-end in an afternoon and then split up
per teammate.

## Architecture

```
                     ┌──────────────────┐
  mock_agent.py ───► │ metrics_service  │ :5001   (Prometheus-style points)
 (fake microservices)│  in-memory dict  │
                     └──────────────────┘
                     ┌──────────────────┐
  mock_agent.py ───► │  logs_service    │ :5002   (LogStream-style)
                     │  in-memory list  │
                     └──────────────────┘
                     ┌──────────────────┐
  mock_agent.py ───► │  traces_service  │ :5003   (OpenTelemetry-style spans)
                     │  in-memory list  │
                     └──────────────────┘
                              ▲
                              │ queries all three for the same time window
                     ┌──────────────────┐
                     │ correlation_svc  │ :5004
                     │ (anomaly + AI)   │
                     └──────────────────┘
```

* **metrics_service.py** — ingests `{service, metric, value, timestamp}`
  points, stores them keyed by `(service, metric)`. Query by service +
  metric + time range.
* **logs_service.py** — ingests `{service, level, message, timestamp,
  trace_id}` lines. Query by service + level + time range. This is the
  piece LogStream already covers.
* **traces_service.py** — ingests spans `{trace_id, span_id,
  parent_span_id, service, operation, start_ts, duration_ms, error}`.
  Can return the slow/erroring spans for a service in a window, or
  reconstruct a full trace by `trace_id`.
* **correlation_service.py** — the interesting piece, and the only one of
  the four that knows the other three exist. It runs a **background
  watcher thread** (started the moment the process boots) that polls
  every ~1.5s, compares the last few seconds of each watched metric
  against a rolling baseline computed with **median + MAD** (median
  absolute deviation — robust because the spike itself is inside the
  sample, so a plain mean/stdev gets dragged toward it and can hide the
  anomaly), and prints two live events as they happen:

  ```
  ANOMALY STARTED  -  checkout-service / latency_ms   (baseline ~76, now 1035)
  ...
  ANOMALY RESOLVED  -  checkout-service / latency_ms
    Duration: 9.5s    Window: 09:18:47 - 09:18:57
    Peak: 1093.3    Baseline: 89.7
    Correlated evidence: 6 slow traces, 9 error/warn logs in this window
    Sample slow trace(s): ...
    Sample log(s): ...
  ```

  When it resolves an incident, it queries the logs and traces services
  **for that exact window** — that's the correlation step: one query that
  says "show me everything else that happened during this specific
  spike," instead of three disconnected signals you'd have to line up
  yourself. It also still exposes a `POST /analyze` endpoint for
  one-shot, on-demand analysis if you'd rather script against it directly
  than watch it live.

* **mock_agent.py** — plays 3 fake microservices (`checkout-service`,
  `payment-service`, `inventory-service`). Sends ~30 ticks of normal
  traffic, then deliberately injects an 8-tick "incident" (checkout latency
  jumps ~10x, payment/checkout start throwing errors), then 10 more ticks
  of recovery, then calls `/analyze` and prints the resulting alert.

## Run it

```bash
pip install flask requests
```

**Option A — one terminal, merged live view (recommended for a demo):**

```bash
./run_demo.sh
```

This starts all 4 services as independent background processes, streams
their live output into your one terminal (each line is already tagged
`METRIC` / `LOG` / `SPAN` / by the service that printed it, plus the
correlation engine's own `ANOMALY STARTED` / `ANOMALY RESOLVED` lines),
waits a beat, then runs the traffic generator at 1 request/second so you
can watch normal traffic → the injected incident → the correlation engine
noticing and reporting on it, all live. Ctrl+C or letting it finish both
clean up the background services properly.

**Option B — one terminal per service (closest to "these are actually 4
independent servers"):**

```bash
# terminal 1
python3 metrics_service.py
# terminal 2
python3 logs_service.py
# terminal 3
python3 traces_service.py
# terminal 4
python3 correlation_service.py       # starts watching immediately, prints alerts on its own
# terminal 5, once the other 4 are up
python3 mock_agent.py
```

Each service is a completely standalone process that only knows about
itself — `metrics_service.py` has never heard of `logs_service.py`. The
correlation service is the only one that talks to the other three, and
only over plain HTTP, exactly like a real Datadog-style engine would.

Want a faster or slower demo? The traffic generator is tunable via env vars:

```bash
TICK_SLEEP=0.3 NORMAL_TICKS_BEFORE=6 INCIDENT_TICKS=4 NORMAL_TICKS_AFTER=6 python3 mock_agent.py
```

Useful endpoints once things are running:

```bash
curl "http://localhost:5001/metrics/query?service=checkout-service&metric=latency_ms&start=0&end=9999999999"
curl "http://localhost:5002/logs/query?service=checkout-service&level=ERROR"
curl "http://localhost:5003/traces/query?service=checkout-service&min_duration=200"
curl -X POST http://localhost:5004/analyze -H "Content-Type: application/json" \
  -d '{"service":"checkout-service","metric":"latency_ms","z_threshold":3.0}'
```

## How this maps to a real final-year project / team split

- **Person A — Metrics pipeline**: swap the in-memory dict for a real
  time-series store (InfluxDB/TimescaleDB), add a `/metrics/scrape`
  Prometheus-compatible pull endpoint instead of push, add downsampling.
- **Person B — Log pipeline**: this is your existing LogStream work — plug
  it in here as `logs_service`, e.g. give it a proper index/search.
- **Person C — Tracing**: replace hand-rolled spans with real OpenTelemetry
  SDK instrumentation in a couple of toy services, use OTLP ingestion
  instead of the custom `/spans` endpoint.
- **Person D — Correlation/AI layer**: this is the most extensible part —
  swap median/MAD for a proper model (seasonal decomposition, isolation
  forest, or even a small LLM call that reads the window's logs+traces and
  writes the human-readable incident summary). Add a `/watch` endpoint that
  polls continuously instead of only analyzing on-demand.
- **Frontend** (left out here on purpose per your ask): once the above is
  solid, put a dashboard on top — 3 pillar views + a "correlated incidents"
  feed. The correlation endpoint already returns clean JSON, so this is a
  thin layer.

## Notes on the anomaly detector

It uses **median + MAD** (median absolute deviation) rather than mean/std
because in a short window the "baseline" and the "anomaly" are the same
sample — a couple of extreme points pull a plain mean/std z-score toward
themselves and can hide the anomaly. MAD is far less sensitive to those
few extreme points, so the spike still stands out clearly even with only
~50 data points, which is realistic for a demo/prototype scale. At
production scale you'd fit the baseline from a separate, larger historical
window instead.
