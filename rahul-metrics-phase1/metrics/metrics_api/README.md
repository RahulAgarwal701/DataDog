# metrics-api (Rahul) - :9001

Translates Prometheus into the contract's `MetricSeries` / `MetricSnapshot`.

| Endpoint | Returns |
|---|---|
| `GET /api/v1/metrics/query?service=&metric=&start=&end=&step_seconds=` | `MetricSeries` (default last 15 min, step 5, max 2000 points) |
| `GET /api/v1/metrics/latest?service=` | `list[MetricSnapshot]` (service optional = all) |
| `GET /health` | `HealthResponse` |

## Run

```bash
cd metrics/metrics_api
pip install -r requirements-dev.txt
pip install -e ../../contracts/python      # Darsan's package. Until it lands, a local stub subset is used automatically.

MOCK=true uvicorn app.main:app --port 9001                               # no Prometheus needed
PROMETHEUS_URL=http://localhost:9090 uvicorn app.main:app --port 9001   # real
```
Try: `curl 'localhost:9001/api/v1/metrics/query?service=payments&metric=latency_p95_ms'` and `curl localhost:9001/api/v1/metrics/latest`.

## Prometheus
- Compose targets: `../prometheus/prometheus.yml` · native binary on the host: `../prometheus/prometheus.local.yml`
- `docker compose -f metrics/docker-compose.metrics.yml up --build` starts Prometheus + this API (Darsan merges it into `deploy/`).

## Test
```bash
python -m unittest discover -s tests -t . -p "test_core.py"   # stdlib only, uses a fake Prometheus
pytest tests/                                                  # adds test_contract.py (validates every response with the pydantic models)
python ../scripts/verify_metrics.py                            # against the live stack: targets UP + 5x4 metrics non-null
python ../scripts/verify_metrics.py --watch payments           # watch p95 react to a fault
```

## Layout
`app/promql.py` exact contract PromQL · `app/prom_client.py` Prometheus HTTP client · `app/service.py` validation, gaps, limits · `app/mock.py` mock data · `app/main.py` FastAPI glue and error handlers.
