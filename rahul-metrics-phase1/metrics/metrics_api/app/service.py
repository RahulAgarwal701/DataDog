"""Business logic (framework-free). main.py is only HTTP glue around this."""
from concurrent.futures import ThreadPoolExecutor
from datetime import datetime, timedelta

from .config import Settings
from .errors import ApiError
from .mock import mock_value
from .prom_client import PrometheusClient, PrometheusError, PrometheusUnavailable
from .promql import METRICS, UNITS, build_query
from .timefmt import ensure_utc, fmt_ts, from_ms, to_ms, utcnow

MAX_POINTS = 2000
DEFAULT_WINDOW_S = 900
DEFAULT_STEP_S = 5


class MetricsService:
    def __init__(self, settings: Settings, client: PrometheusClient | None = None):
        self.s = settings
        self.client = client or PrometheusClient(settings.prometheus_url, settings.prom_timeout)

    # ---------- validation ----------
    def check_service(self, service: str) -> None:
        if service not in self.s.services:
            raise ApiError("not_found", f"unknown service '{service}'", {"known_services": self.s.services})

    @staticmethod
    def check_metric(metric: str) -> None:
        if metric not in METRICS:
            raise ApiError("invalid_argument", f"unknown metric '{metric}'", {"allowed": METRICS})

    def plan_window(self, start: datetime | None, end: datetime | None, step: int):
        """Applies defaults (last 15 min, step 5), validates, returns (start_ms, end_ms, step)."""
        if step < 1 or step > 3600:
            raise ApiError("invalid_argument", "step_seconds must be between 1 and 3600")
        step_ms = step * 1000
        if end is None:
            end_ms = (to_ms(utcnow()) // step_ms) * step_ms          # align to the grid
        else:
            end_ms = to_ms(ensure_utc(end))
        start_ms = to_ms(ensure_utc(start)) if start else end_ms - DEFAULT_WINDOW_S * 1000
        if start_ms >= end_ms:
            raise ApiError("invalid_argument", "start must be before end")
        n_points = (end_ms - start_ms) // step_ms + 1
        if n_points > MAX_POINTS:
            raise ApiError("invalid_argument",
                           f"{n_points} points requested; max is {MAX_POINTS}. Increase step_seconds or narrow the range.",
                           {"points": n_points, "max_points": MAX_POINTS})
        return start_ms, end_ms, step

    # ---------- query ----------
    def query(self, service: str, metric: str, start: datetime | None, end: datetime | None, step: int) -> dict:
        self.check_service(service)
        self.check_metric(metric)
        start_ms, end_ms, step = self.plan_window(start, end, step)
        step_ms = step * 1000
        grid = list(range(start_ms, end_ms + 1, step_ms))

        if self.s.mock:
            values = {t: mock_value(service, metric, t / 1000.0, self.s.mock_fault_cycle) for t in grid}
        else:
            try:
                samples = self.client.query_range(build_query(metric, service), start_ms / 1000, end_ms / 1000, step)
            except PrometheusUnavailable as e:
                raise ApiError("upstream_unavailable", str(e)) from e
            except PrometheusError as e:
                raise ApiError("upstream_unavailable", f"Prometheus rejected the query: {e}") from e
            values = {int(round(ts * 1000)): v for ts, v in samples}

        return {
            "service": service,
            "metric": metric,
            "unit": UNITS[metric],
            "step_seconds": step,
            "start": fmt_ts(from_ms(start_ms)),
            "end": fmt_ts(from_ms(end_ms)),
            # gaps (no sample at a grid point, or NaN) are null per contract
            "points": [{"ts": fmt_ts(from_ms(t)), "value": values.get(t)} for t in grid],
        }

    # ---------- latest ----------
    def latest(self, service: str | None) -> list[dict]:
        if service:
            self.check_service(service)
        services = [service] if service else list(self.s.services)
        now = utcnow()
        ts = now.timestamp()

        if self.s.mock:
            results = {(sv, m): mock_value(sv, m, ts, self.s.mock_fault_cycle) for sv in services for m in METRICS}
        else:
            pairs = [(sv, m) for sv in services for m in METRICS]

            def one(p):
                return p, self.client.query_instant(build_query(p[1], p[0]))

            try:
                with ThreadPoolExecutor(max_workers=10) as ex:
                    results = dict(ex.map(one, pairs))
            except PrometheusUnavailable as e:
                raise ApiError("upstream_unavailable", str(e)) from e
            except PrometheusError as e:
                raise ApiError("upstream_unavailable", f"Prometheus rejected the query: {e}") from e

        return [{"service": sv, "timestamp": fmt_ts(now),
                 "values": {m: results[(sv, m)] for m in METRICS}} for sv in services]

    # ---------- health ----------
    def healthy(self) -> bool:
        return True if self.s.mock else self.client.ready()
