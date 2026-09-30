import os
os.environ["MOCK"] = "true"
from fastapi.testclient import TestClient
from logs.logs_api.main import app
from logs.logs_api._models import LogRecord, LogStats, Page, HealthResponse

c = TestClient(app)

def test_health(): HealthResponse.model_validate(c.get("/health").json())

def test_search_filters():
    r = c.get("/api/v1/logs/search", params={"service": "payments", "severity": ["ERROR", "WARN"], "limit": 5}).json()
    p = Page[LogRecord].model_validate(r)
    assert p.items and all(i.service == "payments" and i.severity.value in ("ERROR", "WARN") for i in p.items)

def test_around_and_stats():
    ts = c.get("/api/v1/logs/search", params={"limit": 1}).json()["items"][0]["timestamp"]
    Page[LogRecord].model_validate(c.get("/api/v1/logs/around", params={"timestamp": ts}).json())
    LogStats.model_validate(c.get("/api/v1/logs/stats").json())

def test_bad_arg():
    r = c.get("/api/v1/logs/search", params={"limit": 99999})
    assert r.status_code == 400 and r.json()["error"]["code"] == "invalid_argument"
