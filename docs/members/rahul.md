# Rahul — metrics pipeline + Metrics API

**Folder you own:** `rahul-metrics-phase1/metrics/`
**Ports:** 9001 (Metrics API), 9090 (Prometheus)
**Phase 2/3:** retries, `ingest` endpoint, baseline-friendly queries for the detector,
API latency benchmarks, retention/config, resource overhead measurement

## What you built, in one paragraph

Prometheus scrapes the four services' `/metrics` endpoints every 5 seconds. The Metrics
API turns PromQL into the contract's two endpoints — a `latest` snapshot per service and a
`query` over a time range — validating the service name against the registry and the metric
name against a fixed allow-list. Prometheus owns the `service` label, not the services, so
the label is consistent across every series.

## The deliverables

| Path | What it is |
|---|---|
| `metrics_api/app/promql.py` | the canonical metric → PromQL map, straight from contract 4.2 |
| `metrics_api/app/service.py` | validation, window planning, gridding, gap handling |
| `metrics_api/app/prom_client.py` | the Prometheus HTTP client |
| `metrics_api/app/config.py`, `errors.py`, `timefmt.py` | config, error contract, time handling |
| `metrics_api/app/mock.py` | `MOCK=true` fixture generator |
| `prometheus/prometheus.yml` | 5s scrape, one job per service, `service` label attached here |
| `scripts/verify_metrics.py` | standalone check against a live Prometheus |

**Routes:** `GET /health`, `GET /api/v1/metrics/latest`, `GET /api/v1/metrics/query`
**Metrics:** `request_rate`, `latency_p95_ms`, `error_rate`, `cpu_percent`, `memory_mb`

## The two design decisions worth defending

**Prometheus owns the `service` label.** It is attached in `prometheus.yml`, not exported
by each service. Otherwise every service has to label itself consistently, and one
mismatch silently splits a series in two. The cost is that a service scraped outside
compose needs the label added to its scrape config.

**Gaps are `null`, never zero.** If Prometheus has no sample at a grid point — service
down, before the first scrape, NaN — the point's `value` is `null`. This is the single
most important decision in your component. A zero says "we measured and it was nothing";
a null says "we could not measure". Collapsing them makes a dead service look like an idle
one, and it is exactly what the Phase 2 anomaly detector would build false positives on.

It is live right now and you can show it:

```bash
curl -s "localhost:9000/api/v1/metrics/query?service=payments&metric=error_rate&start=...&end=...&step_seconds=15"
```

returns 13 points, all `null` — because payments has had no 5xx series, and the ratio
query has no numerator. That is the gap rule working, not a failure.

## How to demo it

```bash
make -C demo preflight        # confirms request_rate is flowing on all four
```

Metrics tab in the dashboard, or raw:

```bash
curl -s localhost:9090/api/v1/targets   # 4 targets, all "up"
curl -s localhost:9001/api/v1/metrics/latest | python3 -m json.tool
```

Then `make -C demo fault-latency` and come back in 15 seconds: measured, p95 goes from
**~98 ms to ~952 ms** with payments faulted at +800 ms. Note it overshoots 800 because the
faulted hop is on top of the real baseline latency of the other three hops — it is additive
delay on top of a real request, not a synthetic number.

## Questions you will actually get

**"How do you compute p95?"**

`histogram_quantile(0.95, sum by (le) (rate(http_request_duration_seconds_bucket[30s])))`,
times 1000. Worth volunteering the limitation: that is an estimate interpolated between
bucket boundaries, so its precision is bounded by your bucket width. It is not exact, and
it is not comparable across services with different bucket layouts. The contract pins the
buckets so they are.

**"Why a 30s rate window?"**

`rate(...[30s])` needs enough samples to be stable. At a 5s scrape that is six samples. The
trade-off is real in both directions: a shorter window tracks faults faster but gets noisy
at 5 rps, a longer one is smoother but lags. This is why the latency fault takes ~15s to
show up and why the graph takes ~30s longer to settle after the fault clears. If you are
asked why the dashboard is slow to react, that is the answer.

**"How do you handle a service that has never been scraped?"**

`latest` reports it as a gap rather than as zero, and a service whose `up` is 0 surfaces as
`upstream_unavailable` (503) rather than as healthy-with-no-traffic. The consequence is
deliberate: a brand-new service has to be scraped before it reads as healthy, so "no data"
never masquerades as "all good".

**"What is your error rate, exactly?"**

`sum(rate(http_requests_total{status=~"5.."}[30s])) / clamp_min(sum(rate(http_requests_total[30s])), 0.0001)`.
The `clamp_min` stops a division by zero when a service has no traffic. Note the
denominator is per-service total, so a downstream 5xx counts against the *calling*
service's error rate too. That is intentional — it is what makes the cascade visible — but
it means an error rate is not attributable to that service's own code. Worth saying before
you are asked.

**"MOCK mode — is the demo faked?"**

No. `MOCK=true` exists so the dashboard could be built before Prometheus existed, and so
the API can be demonstrated without a Prometheus. It is off. All four Prometheus targets
are `up` and the numbers on screen come from live scrapes.

## Things that will trip you up

- **The error rate *falls* during the refusal fault.** Prometheus is on a 30s window that
  still holds the 500s from the preceding error fault, so the number decays rather than
  spiking. Do not point at the error-rate panel during the refusal step — point at the red
  topology edge and the `Errno 111` log line instead. The decline is a measurement artefact
  of window overlap, not a recovery.
- **`cpu_percent` and `memory_mb` come from `process_*`**, which is per-process, not
  per-container. Four uvicorn workers would report the same process twice. Fine for one
  process per service, wrong the moment anyone scales out.
- **Prometheus needs ~20s on first boot** before the Metrics tab shows anything. If the
  tab is empty right after `make demo`, that is why.

## Your own commands

```bash
# 18 passed, 1 skipped -- verified. The image already has the runtime deps, so
# this is the shortest honest way to run them:
docker run --rm --network minidd_default \
  -v "$PWD/darsan/contracts/python:/contracts/python:ro" \
  -v "$PWD/rahul-metrics-phase1/metrics:/m" -w /m/metrics_api \
  "$(docker inspect minidd-metrics-api-1 --format '{{.Config.Image}}')" \
  sh -c 'pip install -q pytest; python -m pytest tests/ -q'

# or locally: pip install -r requirements-dev.txt && python -m pytest tests/ -q
python scripts/verify_metrics.py        # against a live Prometheus
MOCK=true python -m app.main            # no Prometheus needed
cat rahul-metrics-phase1/docs/metrics.md   # your own design notes
docker compose -f docker-compose.metrics.yml up -d   # metrics alone, no full stack
```

Note `pytest` is a **dev-only** dependency (`requirements-dev.txt`), deliberately not in the
runtime image — the metrics-api container itself has no pytest. If someone asks you to run
the tests inside the running container, that is why it fails, and it is correct behaviour,
not a broken setup.
