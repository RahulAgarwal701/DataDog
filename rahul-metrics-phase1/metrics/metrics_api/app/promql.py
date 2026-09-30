"""Canonical metrics -> exact PromQL from contracts section 4.2. ($S = service). Do not edit without a contract change."""

METRICS = ["request_rate", "latency_p95_ms", "error_rate", "cpu_percent", "memory_mb"]

UNITS = {
    "request_rate": "req/s",
    "error_rate": "ratio",
    "latency_p95_ms": "ms",
    "cpu_percent": "percent",
    "memory_mb": "MB",
}

PROMQL = {
    "request_rate": 'sum(rate(http_requests_total{service="$S"}[30s]))',
    "error_rate": (
        'sum(rate(http_requests_total{service="$S",status=~"5.."}[30s])) / '
        'clamp_min(sum(rate(http_requests_total{service="$S"}[30s])), 0.0001)'
    ),
    "latency_p95_ms": (
        '1000 * histogram_quantile(0.95, sum by (le) '
        '(rate(http_request_duration_seconds_bucket{service="$S"}[30s])))'
    ),
    "cpu_percent": '100 * rate(process_cpu_seconds_total{service="$S"}[30s])',
    "memory_mb": 'process_resident_memory_bytes{service="$S"} / 1048576',
}


def build_query(metric: str, service: str) -> str:
    # `service` is validated against the registry before this is called (no injection surface).
    return PROMQL[metric].replace("$S", service)
