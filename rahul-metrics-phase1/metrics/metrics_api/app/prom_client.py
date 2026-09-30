"""Tiny Prometheus HTTP client (stdlib only)."""
import json
import math
import urllib.error
import urllib.parse
import urllib.request


class PrometheusUnavailable(Exception):
    """Prometheus unreachable / timed out / 5xx."""


class PrometheusError(Exception):
    """Prometheus answered but rejected the query."""


def _num(v) -> float | None:
    try:
        f = float(v)
    except (TypeError, ValueError):
        return None
    return None if (math.isnan(f) or math.isinf(f)) else round(f, 4)


class PrometheusClient:
    def __init__(self, base_url: str, timeout: float = 5.0):
        self.base = base_url.rstrip("/")
        self.timeout = timeout

    def _get(self, path: str, params: dict | None = None) -> dict:
        url = self.base + path + ("?" + urllib.parse.urlencode(params) if params else "")
        try:
            with urllib.request.urlopen(url, timeout=self.timeout) as r:
                body = r.read()
        except urllib.error.HTTPError as e:
            if e.code >= 500:
                raise PrometheusUnavailable(f"Prometheus returned HTTP {e.code}") from e
            raise PrometheusError(e.read().decode("utf-8", "replace")[:300]) from e
        except (urllib.error.URLError, TimeoutError, ConnectionError, OSError) as e:
            raise PrometheusUnavailable(f"cannot reach Prometheus at {self.base}: {e}") from e
        try:
            data = json.loads(body)
        except ValueError as e:
            raise PrometheusError("Prometheus returned non-JSON") from e
        if data.get("status") != "success":
            raise PrometheusError(str(data.get("error", "unknown error")))
        return data["data"]

    def ready(self) -> bool:
        try:
            with urllib.request.urlopen(self.base + "/-/ready", timeout=min(self.timeout, 2)) as r:
                return r.status == 200
        except Exception:
            return False

    def query_range(self, query: str, start_s: float, end_s: float, step_s: int) -> list[tuple[float, float | None]]:
        """Returns [(unix_ts, value|None)] for the first series ([] if the query matched nothing)."""
        data = self._get("/api/v1/query_range", {"query": query, "start": f"{start_s:.3f}",
                                                 "end": f"{end_s:.3f}", "step": f"{step_s}s"})
        result = data.get("result", [])
        if not result:
            return []
        return [(float(ts), _num(v)) for ts, v in result[0].get("values", [])]

    def query_instant(self, query: str) -> float | None:
        data = self._get("/api/v1/query", {"query": query})
        result = data.get("result", [])
        if not result:
            return None
        return _num(result[0]["value"][1])
