"""MOCK=true data: deterministic, fixture-shaped, no Prometheus needed. Lets Darsan build the UI early."""
import math
import random
import zlib

BASE_LATENCY_MS = {"inventory": 22.0, "payments": 54.0, "orders": 85.0, "api-gateway": 100.0}
BASE_MEMORY_MB = {"inventory": 62.0, "payments": 68.0, "orders": 71.0, "api-gateway": 79.0}
FAULT_PERIOD_S = 180
FAULT_START_S, FAULT_END_S = 120, 150     # 30 s of +800 ms on payments (and everything upstream of it)
FAULT_EXTRA_MS = {"payments": 800.0, "orders": 800.0, "api-gateway": 800.0}


def _noise(service: str, metric: str, ts: float) -> float:
    """Deterministic value in [-1, 1] per (service, metric, 5s bucket)."""
    seed = zlib.crc32(f"{service}:{metric}:{int(ts // 5)}".encode())
    return random.Random(seed).uniform(-1.0, 1.0)


def _in_fault(ts: float, cycle: bool) -> bool:
    return cycle and FAULT_START_S <= (ts % FAULT_PERIOD_S) < FAULT_END_S


def mock_value(service: str, metric: str, ts: float, fault_cycle: bool = True) -> float:
    n = _noise(service, metric, ts)
    if metric == "request_rate":
        return round(max(0.0, 5.0 + 0.4 * n), 4)
    if metric == "latency_p95_ms":
        v = BASE_LATENCY_MS.get(service, 50.0) * (1 + 0.08 * n)
        if _in_fault(ts, fault_cycle):
            v += FAULT_EXTRA_MS.get(service, 0.0)
        return round(v, 4)
    if metric == "error_rate":
        return round(max(0.0, 0.004 + 0.004 * n), 4)
    if metric == "cpu_percent":
        return round(max(0.0, 4.0 + 1.2 * n), 4)
    if metric == "memory_mb":
        drift = 2.0 * math.sin(ts / 600.0)
        return round(BASE_MEMORY_MB.get(service, 70.0) + drift + 0.2 * n, 4)
    raise KeyError(metric)
