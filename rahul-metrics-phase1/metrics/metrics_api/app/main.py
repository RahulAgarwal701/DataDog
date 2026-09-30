"""Metrics API :9001 (Rahul). Endpoints per contracts section 7.2 + GET /health."""
import logging
from datetime import datetime
from typing import Optional

from fastapi import FastAPI, Query, Request
from fastapi.exceptions import RequestValidationError
from fastapi.middleware.cors import CORSMiddleware
from fastapi.responses import JSONResponse
from starlette.exceptions import HTTPException as StarletteHTTPException

from minidd_contracts.models import HealthResponse, MetricSeries, MetricSnapshot

from .config import Settings
from .errors import ApiError
from .service import DEFAULT_STEP_S, MetricsService
from .timefmt import fmt_ts, utcnow

settings = Settings()
logging.basicConfig(level=settings.log_level.upper(), format="%(asctime)s %(levelname)s %(name)s %(message)s")
log = logging.getLogger("metrics-api")
svc = MetricsService(settings)

app = FastAPI(title="Mini Datadog - Metrics API", version="1.0.0")
app.add_middleware(CORSMiddleware, allow_origins=["*"], allow_methods=["*"], allow_headers=["*"])


# ---------- errors: every non-2xx body is ErrorResponse ----------
def _err(code: str, status: int, message: str, details: dict | None = None) -> JSONResponse:
    e = ApiError(code, message, details)
    return JSONResponse(status_code=status, content=e.body())


@app.exception_handler(ApiError)
async def _api_error(_: Request, exc: ApiError):
    return JSONResponse(status_code=exc.status, content=exc.body())


@app.exception_handler(RequestValidationError)
async def _validation(_: Request, exc: RequestValidationError):
    errs = [{"loc": ".".join(str(x) for x in e["loc"]), "msg": e["msg"]} for e in exc.errors()]
    return _err("invalid_argument", 400, "invalid request parameters", {"errors": errs})


@app.exception_handler(StarletteHTTPException)
async def _http(_: Request, exc: StarletteHTTPException):
    code = "not_found" if exc.status_code == 404 else "invalid_argument" if exc.status_code < 500 else "internal_error"
    return _err(code, exc.status_code, str(exc.detail))


@app.exception_handler(Exception)
async def _unhandled(_: Request, exc: Exception):
    log.exception("unhandled error")
    return _err("internal_error", 500, "internal error")


# ---------- endpoints (sync defs run in FastAPI's threadpool; Prometheus calls are blocking) ----------
@app.get("/health")
def health():
    data = {"service": "metrics-api", "status": "ok" if svc.healthy() else "degraded",
            "version": "1.0.0", "timestamp": fmt_ts(utcnow())}
    HealthResponse.model_validate(data)
    return data


@app.get("/api/v1/metrics/query")
def query(service: str = Query(...), metric: str = Query(...),
          start: Optional[datetime] = None, end: Optional[datetime] = None,
          step_seconds: int = DEFAULT_STEP_S):
    data = svc.query(service, metric, start, end, step_seconds)
    MetricSeries.model_validate(data)        # contract check on every response
    return data


@app.get("/api/v1/metrics/latest")
def latest(service: Optional[str] = None):
    data = svc.latest(service)
    for d in data:
        MetricSnapshot.model_validate(d)
    return data


@app.on_event("startup")
def _startup():
    log.info("metrics-api up: mock=%s prometheus=%s services=%s",
             settings.mock, settings.prometheus_url, settings.services)
