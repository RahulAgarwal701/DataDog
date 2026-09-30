"""Logs API :9002 - /api/v1/logs/{search,around,stats}, /health. MOCK=true serves in-memory data."""
import os, random
from datetime import datetime, timedelta, timezone
from typing import Optional
import httpx
from fastapi import FastAPI, Query, Request
from fastapi.exceptions import RequestValidationError
from fastapi.middleware.cors import CORSMiddleware
from fastapi.responses import JSONResponse
try:
    from minidd_contracts.models import Base, Severity, HealthResponse, ErrorBody, ErrorResponse, Page, LogRecord, LogStats
except ImportError:
    from ._models import Base, Severity, HealthResponse, ErrorBody, ErrorResponse, Page, LogRecord, LogStats

ES_URL = os.getenv("ES_URL", "http://localhost:9200")
INDEX = os.getenv("ES_INDEX", "minidd-logs")
MOCK = os.getenv("MOCK", "false").lower() == "true"
SEV_ORDER = ["DEBUG", "INFO", "WARN", "ERROR"]
SERVICES = ["api-gateway", "orders", "payments", "inventory"]

app = FastAPI(title="logs-api")
app.add_middleware(CORSMiddleware, allow_origins=["*"], allow_methods=["*"], allow_headers=["*"])


class ApiError(Exception):
    def __init__(self, status, code, msg): self.status, self.code, self.msg = status, code, msg


def err(status, code, msg):
    return JSONResponse(status_code=status, content=ErrorResponse(error=ErrorBody(code=code, message=msg)).model_dump(mode="json"))

@app.exception_handler(ApiError)
async def _a(_: Request, e: ApiError): return err(e.status, e.code, e.msg)

@app.exception_handler(RequestValidationError)
async def _v(_: Request, e): return err(400, "invalid_argument", str(e.errors()[0]["msg"]) + " @ " + ".".join(map(str, e.errors()[0]["loc"])))

@app.exception_handler(Exception)
async def _e(_: Request, e): return err(500, "internal_error", str(e))


def now(): return datetime.now(timezone.utc)
def utc(d: datetime): return d if d.tzinfo else d.replace(tzinfo=timezone.utc)
def iso(d: datetime):
    d = utc(d).astimezone(timezone.utc)
    return d.strftime("%Y-%m-%dT%H:%M:%S.") + f"{d.microsecond // 1000:03d}Z"


# ---------------- mock data ----------------
_MOCK: list = []
if MOCK:
    t0 = now() - timedelta(minutes=30)
    for i in range(600):
        s = random.choice(SERVICES); rid = f"req_{random.getrandbits(64):016x}"
        slow = s == "payments" and i > 450
        sev = "ERROR" if slow and i % 3 == 0 else "WARN" if slow else "INFO"
        msg = {"ERROR": "downstream call failed", "WARN": "slow request", "INFO": "request completed"}[sev]
        _MOCK.append(LogRecord(id=str(i), timestamp=t0 + timedelta(seconds=i * 3), service=s, severity=sev, message=msg,
                               request_id=rid, fields={"path": "/charge", "status": 500 if sev == "ERROR" else 200, "latency_ms": 900 if slow else 50}))


def mock_filter(services, sevs, q, start, end):
    r = _MOCK
    if services: r = [x for x in r if x.service in services]
    if sevs: r = [x for x in r if x.severity.value in sevs]
    if q: r = [x for x in r if q.lower() in x.message.lower()]
    if start: r = [x for x in r if utc(x.timestamp) >= utc(start)]
    if end: r = [x for x in r if utc(x.timestamp) <= utc(end)]
    return r


# ---------------- ES helpers ----------------
async def es(path, body):
    try:
        async with httpx.AsyncClient(timeout=10) as c:
            r = await c.post(f"{ES_URL}/{INDEX}/{path}", json=body)
    except httpx.HTTPError as e:
        raise ApiError(503, "upstream_unavailable", f"Elasticsearch unreachable: {e}")
    if r.status_code == 404:  # index not created yet (no logs shipped)
        return {"hits": {"total": {"value": 0}, "hits": []}, "aggregations": {}}
    if r.status_code >= 400:
        raise ApiError(503, "upstream_unavailable", f"Elasticsearch error {r.status_code}")
    return r.json()


def build_query(services, sevs, q, start, end):
    f = []
    if services: f.append({"terms": {"service": services}})
    if sevs: f.append({"terms": {"severity": sevs}})
    if start or end:
        rng = {}
        if start: rng["gte"] = iso(start)
        if end: rng["lte"] = iso(end)
        f.append({"range": {"timestamp": rng}})
    must = [{"match": {"message": q}}] if q else []
    return {"bool": {"filter": f, "must": must}}


def to_rec(h): return LogRecord(id=h["_id"], **h["_source"])


async def run_search(services, sevs, q, start, end, limit, offset, order):
    if MOCK:
        r = sorted(mock_filter(services, sevs, q, start, end), key=lambda x: x.timestamp, reverse=(order == "desc"))
        return Page[LogRecord](items=r[offset:offset + limit], total=len(r), limit=limit, offset=offset)
    d = await es("_search", {"query": build_query(services, sevs, q, start, end), "from": offset, "size": limit,
                             "sort": [{"timestamp": order}], "track_total_hits": True})
    return Page[LogRecord](items=[to_rec(h) for h in d["hits"]["hits"]], total=d["hits"]["total"]["value"], limit=limit, offset=offset)


# ---------------- endpoints ----------------
@app.get("/health", response_model=HealthResponse)
async def health():
    ok = True
    if not MOCK:
        try:
            async with httpx.AsyncClient(timeout=3) as c:
                ok = (await c.get(f"{ES_URL}/_cluster/health")).status_code == 200
        except httpx.HTTPError:
            ok = False
    return HealthResponse(service="logs-api", status="ok" if ok else "degraded", timestamp=now())


@app.get("/api/v1/logs/search", response_model=Page[LogRecord])
async def search(service: Optional[list[str]] = Query(None), severity: Optional[list[Severity]] = Query(None),
                 q: Optional[str] = None, start: Optional[datetime] = None, end: Optional[datetime] = None,
                 limit: int = Query(100, ge=1, le=1000), offset: int = Query(0, ge=0), order: str = Query("desc", pattern="^(asc|desc)$")):
    return await run_search(service, [s.value for s in severity] if severity else None, q, start, end, limit, offset, order)


@app.get("/api/v1/logs/around", response_model=Page[LogRecord])
async def around(timestamp: datetime, before_seconds: int = Query(60, ge=0), after_seconds: int = Query(30, ge=0),
                 service: Optional[list[str]] = Query(None), min_severity: Severity = Severity.INFO):
    sevs = SEV_ORDER[SEV_ORDER.index(min_severity.value):]
    ts = utc(timestamp)
    return await run_search(service, sevs, None, ts - timedelta(seconds=before_seconds), ts + timedelta(seconds=after_seconds), 1000, 0, "asc")


@app.get("/api/v1/logs/stats", response_model=LogStats)
async def stats(start: Optional[datetime] = None, end: Optional[datetime] = None):
    end = utc(end) if end else now()
    start = utc(start) if start else end - timedelta(hours=1)
    if MOCK:
        bs, bv = {}, {}
        for x in mock_filter(None, None, None, start, end):
            bs[x.service] = bs.get(x.service, 0) + 1
            bv[x.severity] = bv.get(x.severity, 0) + 1
        return LogStats(start=start, end=end, by_service=bs, by_severity=bv)
    d = await es("_search", {"size": 0, "query": build_query(None, None, None, start, end), "aggs": {
        "s": {"terms": {"field": "service", "size": 50}}, "v": {"terms": {"field": "severity", "size": 10}}}})
    a = d.get("aggregations", {})
    return LogStats(start=start, end=end,
                    by_service={b["key"]: b["doc_count"] for b in a.get("s", {}).get("buckets", [])},
                    by_severity={b["key"]: b["doc_count"] for b in a.get("v", {}).get("buckets", [])})


if __name__ == "__main__":
    import uvicorn
    uvicorn.run(app, host="0.0.0.0", port=int(os.getenv("PORT", "9002")), log_level=os.getenv("LOG_LEVEL", "info").lower())
