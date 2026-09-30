# Metrics pipeline: definitions (Rahul)

## Data path
`dummy services (/metrics, Prometheus text)` → **Prometheus** (scrape every 5 s, attaches `service` label) → **Metrics API :9001** (runs the PromQL below, returns contract DTOs) → Platform API :9000 → dashboard.

Services expose `http_requests_total{method,path,status}`, `http_request_duration_seconds{method,path}` (histogram) and the default process metrics. They never set the `service` label; `metrics/prometheus/prometheus.yml` does it per target.

## Canonical metrics

| Metric | Unit | Meaning | Notes |
|---|---|---|---|
| `request_rate` | req/s | Requests per second served by the service, all routes and statuses | 30 s rolling window. Baseline ≈ 5 req/s at loadgen baseline. |
| `error_rate` | ratio 0..1 | Fraction of requests that returned HTTP 5xx | 4xx are not errors. Denominator is clamped, so zero traffic gives 0, not NaN. |
| `latency_p95_ms` | ms | 95th-percentile request duration | Estimated from histogram buckets by linear interpolation, so the precision is bounded by bucket width. `null` when there is no traffic in the window. |
| `cpu_percent` | percent of one core | CPU time consumed per second of wall-clock | Can exceed 100 for multi-threaded processes. |
| `memory_mb` | MB | Resident set size (RSS) of the process | Instantaneous gauge. |

## Exact PromQL (contracts §4.2, `$S` = service)

```
request_rate    sum(rate(http_requests_total{service="$S"}[30s]))
error_rate      sum(rate(http_requests_total{service="$S",status=~"5.."}[30s])) / clamp_min(sum(rate(http_requests_total{service="$S"}[30s])), 0.0001)
latency_p95_ms  1000 * histogram_quantile(0.95, sum by (le) (rate(http_request_duration_seconds_bucket{service="$S"}[30s])))
cpu_percent     100 * rate(process_cpu_seconds_total{service="$S"}[30s])
memory_mb       process_resident_memory_bytes{service="$S"} / 1048576
```

## Behaviour worth knowing (and saying in the viva)
- **Gaps are `null`, never zero.** If Prometheus has no sample for a grid point (service down, before first scrape, NaN), the point's `value` is `null`.
- **Why a 30 s window with a 5 s scrape:** `rate()` needs ≥2 samples; 30 s gives ~6, which smooths noise. The price is lag: a fault shows up gradually and is fully visible about 30 s after injection. That is why the demo says "within ~30 s".
- **Grid alignment:** points are `start + k·step`; default `end` is "now" floored to the step, default window is 15 min, step 5 s, max 2000 points (else HTTP 400).
- **Errors:** unknown metric → 400 `invalid_argument`; unknown service → 404 `not_found`; Prometheus unreachable → 503 `upstream_unavailable`. All bodies are `ErrorResponse`.
- **Retention:** Prometheus keeps 6 h in the demo config (`--storage.tsdb.retention.time=6h`), which is ample for demos and for the Phase 2 baseline window (300 s).
- **Health:** `/health` reports `degraded` if Prometheus is not ready, `ok` otherwise.

## Mock mode
`MOCK=true` serves deterministic, fixture-shaped data with no Prometheus. By default payments latency spikes by +800 ms for 30 s in every 180 s (`MOCK_FAULT_CYCLE=false` disables this), so the dashboard can be developed before the real stack exists.
