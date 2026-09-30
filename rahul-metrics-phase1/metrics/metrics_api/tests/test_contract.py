"""Contract tests: every response validates against minidd_contracts models (extra="forbid").
Needs: pip install -r requirements-dev.txt"""
import os
import sys
import json
import pathlib
import pytest

sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
pytest.importorskip("fastapi"); pytest.importorskip("pydantic"); pytest.importorskip("httpx")

os.environ["MOCK"] = "true"
import app  # noqa: E402,F401  (installs stub path if the real contracts package is absent)
from fastapi.testclient import TestClient  # noqa: E402
from minidd_contracts.models import ErrorResponse, HealthResponse, MetricSeries, MetricSnapshot  # noqa: E402
from app.main import app as fastapi_app  # noqa: E402

client = TestClient(fastapi_app)
SERVICES = ["api-gateway", "orders", "payments", "inventory"]
METRICS = ["request_rate", "latency_p95_ms", "error_rate", "cpu_percent", "memory_mb"]


def test_health():
    r = client.get("/health"); assert r.status_code == 200
    assert HealthResponse.model_validate(r.json()).status == "ok"


@pytest.mark.parametrize("service", SERVICES)
@pytest.mark.parametrize("metric", METRICS)
def test_query_valid_metric_series(service, metric):
    r = client.get("/api/v1/metrics/query", params={"service": service, "metric": metric,
                   "start": "2026-10-14T09:30:00.000Z", "end": "2026-10-14T09:31:00.000Z", "step_seconds": 5})
    assert r.status_code == 200, r.text
    s = MetricSeries.model_validate(r.json())
    assert len(s.points) == 13 and s.step_seconds == 5


def test_query_defaults():
    r = client.get("/api/v1/metrics/query", params={"service": "payments", "metric": "latency_p95_ms"})
    assert r.status_code == 200 and len(r.json()["points"]) == 181


def test_latest_all_and_one():
    r = client.get("/api/v1/metrics/latest"); assert r.status_code == 200
    assert len([MetricSnapshot.model_validate(x) for x in r.json()]) == 4
    r = client.get("/api/v1/metrics/latest", params={"service": "payments"})
    assert len(r.json()) == 1


@pytest.mark.parametrize("params,status,code", [
    ({"service": "payments", "metric": "nope"}, 400, "invalid_argument"),
    ({"service": "nope", "metric": "request_rate"}, 404, "not_found"),
    ({"service": "payments"}, 400, "invalid_argument"),                      # missing metric
    ({"service": "payments", "metric": "request_rate", "start": "garbage"}, 400, "invalid_argument"),
    ({"service": "payments", "metric": "request_rate", "start": "2026-10-14T00:00:00Z",
      "end": "2026-10-14T12:00:00Z", "step_seconds": 5}, 400, "invalid_argument"),   # > 2000 points
])
def test_errors_are_error_response(params, status, code):
    r = client.get("/api/v1/metrics/query", params=params)
    assert r.status_code == status
    assert ErrorResponse.model_validate(r.json()).error.code == code


def test_latest_unknown_service_404():
    r = client.get("/api/v1/metrics/latest", params={"service": "nope"})
    assert r.status_code == 404 and ErrorResponse.model_validate(r.json())


def test_shared_fixture_if_present():
    p = pathlib.Path(__file__).resolve().parents[3] / "contracts" / "fixtures" / "metric_series.json"
    if not p.exists():
        pytest.skip("contracts fixtures not available yet")
    MetricSeries.model_validate(json.loads(p.read_text()))
