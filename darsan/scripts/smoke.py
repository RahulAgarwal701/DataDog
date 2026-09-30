"""make smoke: validates every Platform API response against the contract models."""
import os, sys, httpx
from datetime import datetime, timedelta, timezone
from minidd_contracts.models import *

B = os.getenv("PLATFORM_URL", "http://localhost:9000")
c = httpx.Client(base_url=B, timeout=15)
now = datetime.now(timezone.utc); ts = lambda d: d.strftime("%Y-%m-%dT%H:%M:%S.000Z")
fails = 0

def check(name, path, model, **params):
    global fails
    try:
        r = c.get(path, params=params); r.raise_for_status()
        model.model_validate(r.json()); print("PASS", name)
    except Exception as e:
        fails += 1; print("FAIL", name, "->", str(e)[:150])

for s in ["api-gateway", "orders", "payments", "inventory"]:
    check(f"metrics {s}", "/api/v1/metrics/query", MetricSeries, service=s, metric="latency_p95_ms")
def check_list(name, path, model):
    global fails
    try:
        r = c.get(path); r.raise_for_status()
        from pydantic import TypeAdapter; TypeAdapter(list[model]).validate_python(r.json()); print("PASS", name)
    except Exception as e:
        fails += 1; print("FAIL", name, "->", str(e)[:150])
check_list("metrics latest", "/api/v1/metrics/latest", MetricSnapshot)
check_list("services", "/api/v1/services", ServiceInfo)
check("health all", "/api/v1/health/all", AllHealthResponse)
check("logs search", "/api/v1/logs/search", Page[LogRecord], limit=10)
check("logs stats", "/api/v1/logs/stats", LogStats, start=ts(now - timedelta(hours=1)), end=ts(now))
check("logs around", "/api/v1/logs/around", Page[LogRecord], timestamp=ts(now))
check("network events", "/api/v1/network-events", Page[NetworkEvent], limit=10)
check("topology", "/api/v1/topology", TopologyGraph)
# fault round trip
try:
    r = c.post("/api/v1/demo/fault", json={"service": "payments", "fault": {"type": "latency", "latency_ms": 300, "duration_seconds": 5}})
    FaultConfig.model_validate(r.json()); r.raise_for_status()
    c.post("/api/v1/demo/fault", json={"service": "payments", "fault": {"type": "none"}}); print("PASS demo fault")
except Exception as e:
    fails += 1; print("FAIL demo fault ->", str(e)[:150])
print("SMOKE", "GREEN" if not fails else f"RED ({fails} failed)"); sys.exit(1 if fails else 0)
