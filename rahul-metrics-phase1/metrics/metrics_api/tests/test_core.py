"""Framework-free tests: PromQL, Prometheus client, gap handling, mock mode, limits.
Run: python -m unittest discover -s tests -t .   (or pytest)"""
import os
import sys
import unittest
from datetime import datetime, timezone

sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))

from app.config import Settings
from app.errors import ApiError
from app.mock import mock_value
from app.prom_client import PrometheusClient
from app.promql import METRICS, PROMQL, UNITS, build_query
from app.service import MetricsService
from app.timefmt import fmt_ts
from tests.fake_prom import FakeProm

T0 = datetime(2026, 10, 14, 9, 30, 0, tzinfo=timezone.utc)
T1 = datetime(2026, 10, 14, 9, 30, 20, tzinfo=timezone.utc)
T0_S = T0.timestamp()


def settings(**kw):
    s = Settings()
    for k, v in kw.items():
        setattr(s, k, v)
    return s


class TestPromQL(unittest.TestCase):
    def test_exact_contract_queries(self):
        self.assertEqual(build_query("request_rate", "payments"),
                         'sum(rate(http_requests_total{service="payments"}[30s]))')
        self.assertEqual(build_query("latency_p95_ms", "orders"),
                         '1000 * histogram_quantile(0.95, sum by (le) (rate(http_request_duration_seconds_bucket{service="orders"}[30s])))')
        self.assertEqual(build_query("error_rate", "inventory"),
                         'sum(rate(http_requests_total{service="inventory",status=~"5.."}[30s])) / '
                         'clamp_min(sum(rate(http_requests_total{service="inventory"}[30s])), 0.0001)')
        self.assertEqual(build_query("cpu_percent", "api-gateway"),
                         '100 * rate(process_cpu_seconds_total{service="api-gateway"}[30s])')
        self.assertEqual(build_query("memory_mb", "payments"),
                         'process_resident_memory_bytes{service="payments"} / 1048576')

    def test_all_metrics_have_unit_and_query(self):
        self.assertEqual(set(METRICS), set(PROMQL)); self.assertEqual(set(METRICS), set(UNITS))


class TestTime(unittest.TestCase):
    def test_fmt(self):
        self.assertEqual(fmt_ts(datetime(2026, 10, 14, 9, 30, 1, 412345, tzinfo=timezone.utc)), "2026-10-14T09:30:01.412Z")


class TestValidation(unittest.TestCase):
    def setUp(self):
        self.svc = MetricsService(settings(mock=True))

    def code(self, fn):
        with self.assertRaises(ApiError) as cm:
            fn()
        return cm.exception.code, cm.exception.status

    def test_unknown_service_404(self):
        self.assertEqual(self.code(lambda: self.svc.query("nope", "request_rate", T0, T1, 5)), ("not_found", 404))

    def test_unknown_metric_400(self):
        self.assertEqual(self.code(lambda: self.svc.query("payments", "bogus", T0, T1, 5)), ("invalid_argument", 400))

    def test_too_many_points_400(self):
        far = datetime(2026, 10, 14, 19, 30, 0, tzinfo=timezone.utc)   # 10h @5s = 7201 pts
        self.assertEqual(self.code(lambda: self.svc.query("payments", "request_rate", T0, far, 5)), ("invalid_argument", 400))

    def test_exactly_2000_points_ok(self):
        end = datetime.fromtimestamp(T0_S + 1999 * 5, tz=timezone.utc)
        self.assertEqual(len(self.svc.query("payments", "request_rate", T0, end, 5)["points"]), 2000)

    def test_start_after_end_400(self):
        self.assertEqual(self.code(lambda: self.svc.query("payments", "request_rate", T1, T0, 5)), ("invalid_argument", 400))

    def test_bad_step_400(self):
        self.assertEqual(self.code(lambda: self.svc.query("payments", "request_rate", T0, T1, 0)), ("invalid_argument", 400))

    def test_defaults_last_15_min(self):
        r = self.svc.query("payments", "request_rate", None, None, 5)
        self.assertEqual(len(r["points"]), 181)   # 900s / 5s + 1


class TestMock(unittest.TestCase):
    def test_all_services_metrics_shape(self):
        svc = MetricsService(settings(mock=True))
        for sv in svc.s.services:
            for m in METRICS:
                r = svc.query(sv, m, T0, T1, 5)
                self.assertEqual(r["unit"], UNITS[m]); self.assertEqual(len(r["points"]), 5)
                self.assertTrue(all(isinstance(p["value"], float) for p in r["points"]))
                self.assertTrue(r["start"].endswith("Z") and "." in r["start"])

    def test_latest_all_services(self):
        out = MetricsService(settings(mock=True)).latest(None)
        self.assertEqual([o["service"] for o in out], ["api-gateway", "orders", "payments", "inventory"])
        self.assertEqual(set(out[0]["values"]), set(METRICS))

    def test_deterministic_and_fault_cycle(self):
        self.assertEqual(mock_value("payments", "latency_p95_ms", 1000.0), mock_value("payments", "latency_p95_ms", 1000.0))
        normal = mock_value("payments", "latency_p95_ms", 180 * 10 + 10)      # phase 10  -> baseline
        fault = mock_value("payments", "latency_p95_ms", 180 * 10 + 130)      # phase 130 -> +800ms
        self.assertGreater(fault, normal + 700)
        self.assertLess(mock_value("inventory", "latency_p95_ms", 180 * 10 + 130), 40)   # downstream unaffected


class TestRealPath(unittest.TestCase):
    def test_gaps_nan_and_alignment(self):
        def range_fn(q, s, e, st):
            # 5 grid points 0,5,10,15,20 s; drop t+5, NaN at t+10
            return [(s, 10.5), (s + 10, "NaN"), (s + 15, 12.0), (s + 20, 13.0)]
        with FakeProm(range_fn=range_fn) as fp:
            svc = MetricsService(settings(mock=False), PrometheusClient(fp.url))
            r = svc.query("payments", "latency_p95_ms", T0, T1, 5)
            self.assertEqual([p["value"] for p in r["points"]], [10.5, None, None, 12.0, 13.0])
            self.assertEqual(r["points"][0]["ts"], "2026-10-14T09:30:00.000Z")
            self.assertEqual(r["points"][4]["ts"], "2026-10-14T09:30:20.000Z")
            self.assertEqual(fp.queries[0], build_query("latency_p95_ms", "payments"))

    def test_empty_result_all_null(self):
        with FakeProm(range_fn=lambda *a: None) as fp:
            r = MetricsService(settings(mock=False), PrometheusClient(fp.url)).query("orders", "request_rate", T0, T1, 5)
            self.assertTrue(all(p["value"] is None for p in r["points"]))

    def test_latest_sends_20_queries(self):
        with FakeProm(instant_fn=lambda q: 1.23456789) as fp:
            out = MetricsService(settings(mock=False), PrometheusClient(fp.url)).latest(None)
            self.assertEqual(len(fp.queries), 20)
            self.assertEqual(len(set(fp.queries)), 20)
            self.assertEqual(out[0]["values"]["request_rate"], 1.2346)

    def test_latest_single_service_and_missing_value(self):
        with FakeProm(instant_fn=lambda q: None) as fp:
            out = MetricsService(settings(mock=False), PrometheusClient(fp.url)).latest("payments")
            self.assertEqual(len(out), 1); self.assertIsNone(out[0]["values"]["latency_p95_ms"])

    def test_prometheus_down_is_503(self):
        svc = MetricsService(settings(mock=False), PrometheusClient("http://127.0.0.1:1", timeout=1))
        with self.assertRaises(ApiError) as cm:
            svc.query("payments", "request_rate", T0, T1, 5)
        self.assertEqual((cm.exception.code, cm.exception.status), ("upstream_unavailable", 503))
        with self.assertRaises(ApiError) as cm:
            svc.latest(None)
        self.assertEqual(cm.exception.status, 503)
        self.assertFalse(svc.healthy())


if __name__ == "__main__":
    unittest.main()
