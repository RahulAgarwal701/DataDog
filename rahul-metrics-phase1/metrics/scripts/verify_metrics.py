#!/usr/bin/env python3
"""Phase 1 'done when' checker for Rahul's module (stdlib only).

  python metrics/scripts/verify_metrics.py                # Prometheus targets + 5 metrics x 4 services
  python metrics/scripts/verify_metrics.py --watch payments   # poll latency_p95_ms every 5s (use while injecting a fault)
Env: PROMETHEUS_URL (default http://localhost:9090), METRICS_URL (default http://localhost:9001)
"""
import json, os, sys, time, urllib.request

PROM = os.getenv("PROMETHEUS_URL", "http://localhost:9090")
API = os.getenv("METRICS_URL", "http://localhost:9001")
SERVICES = ["api-gateway", "orders", "payments", "inventory"]
METRICS = ["request_rate", "latency_p95_ms", "error_rate", "cpu_percent", "memory_mb"]


def get(url):
    with urllib.request.urlopen(url, timeout=5) as r:
        return json.loads(r.read())


def check_targets() -> bool:
    try:
        targets = get(f"{PROM}/api/v1/targets")["data"]["activeTargets"]
    except Exception as e:
        print(f"[FAIL] cannot reach Prometheus at {PROM}: {e}"); return False
    up = {t["labels"].get("service"): t["health"] for t in targets}
    ok = True
    for s in SERVICES:
        h = up.get(s, "missing")
        print(f"  [{'OK' if h == 'up' else 'FAIL'}] target {s:<12} {h}")
        ok &= h == "up"
    return ok


def check_api() -> bool:
    try:
        print("  health:", get(f"{API}/health")["status"])
        snaps = {s["service"]: s["values"] for s in get(f"{API}/api/v1/metrics/latest")}
    except Exception as e:
        print(f"[FAIL] metrics-api at {API}: {e}"); return False
    print(f"\n  {'service':<12}" + "".join(f"{m:>16}" for m in METRICS))
    ok = True
    for s in SERVICES:
        vals = snaps.get(s, {})
        print(f"  {s:<12}" + "".join(f"{'null' if vals.get(m) is None else vals[m]:>16}" for m in METRICS))
        ok &= all(vals.get(m) is not None for m in METRICS)
    return ok


def watch(service):
    print(f"watching {service} latency_p95_ms (Ctrl-C to stop)")
    while True:
        s = get(f"{API}/api/v1/metrics/query?service={service}&metric=latency_p95_ms&step_seconds=5&start="
                + time.strftime("%Y-%m-%dT%H:%M:%S.000Z", time.gmtime(time.time() - 60)))
        last = [p for p in s["points"] if p["value"] is not None][-1:]
        print(time.strftime("%H:%M:%S"), last[0]["value"] if last else "no data")
        time.sleep(5)


if __name__ == "__main__":
    if len(sys.argv) > 2 and sys.argv[1] == "--watch":
        watch(sys.argv[2]); sys.exit()
    print("Prometheus targets:"); a = check_targets()
    print("\nMetrics API:"); b = check_api()
    print("\nRESULT:", "PASS" if a and b else "FAIL (values can be null for ~30s after startup; retry)")
    sys.exit(0 if a and b else 1)
