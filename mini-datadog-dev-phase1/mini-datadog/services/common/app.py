"""Shared scaffold: JSONL logs, request-id, Prometheus, fault injection, /health, /metrics, /admin/fault."""
import asyncio, json, os, random, secrets, time
from contextlib import asynccontextmanager
from datetime import datetime, timezone
import httpx
from fastapi import FastAPI, Request
from fastapi.middleware.cors import CORSMiddleware
from fastapi.responses import JSONResponse, Response
from prometheus_client import CONTENT_TYPE_LATEST, Counter, Histogram, generate_latest
from .models import FaultConfig

REQS = Counter("http_requests_total", "requests", ["method", "path", "status"])
DUR = Histogram("http_request_duration_seconds", "latency", ["method", "path"],
                buckets=[0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5])
QUIET = {"/health", "/metrics"}

def now():
    return datetime.now(timezone.utc).isoformat(timespec="milliseconds").replace("+00:00", "Z")

def err(status, code, msg):
    return JSONResponse({"error": {"code": code, "message": msg}}, status_code=status)

class Down(Exception):
    pass

def make_client():
    ka = os.getenv("HTTP_KEEPALIVE", "false").lower() == "true"
    return httpx.AsyncClient(timeout=5, limits=httpx.Limits(max_keepalive_connections=None if ka else 0))

async def call(request: Request, downstream: str, base: str, path: str, body: dict):
    a, rid, t = request.app, request.state.request_id, time.perf_counter()
    try:
        r = await a.state.client.post(base + path, json=body, headers={"X-Request-ID": rid})
        if r.status_code >= 500:
            raise RuntimeError(f"http_{r.status_code}")
        return r.json()
    except Exception as e:
        # httpx buries the real reason ("Connection refused") under a chain of
        # ConnectError/__context__ wrappers that all read "All connection attempts
        # failed". Walk __cause__ and __context__ so the log says *why*.
        cause, seen = e, 0
        while seen < 8:
            nxt = cause.__cause__ or cause.__context__
            if nxt is None or nxt is cause:
                break
            cause, seen = nxt, seen + 1
        why = f"{type(e).__name__}: {e}"
        if cause is not e:
            why += f" -> {type(cause).__name__}: {cause}"
        a.state.log("ERROR", "downstream call failed", rid, downstream=downstream,
                    error=why[:200], latency_ms=round((time.perf_counter() - t) * 1000))
        raise Down(downstream)

def create_app(name: str, port: int, startup=None) -> FastAPI:
    name = os.getenv("SERVICE_NAME", name)
    port = int(os.getenv("PORT", port))
    log_dir = os.getenv("LOG_DIR", "./logs"); os.makedirs(log_dir, exist_ok=True)
    fh = open(f"{log_dir}/{name}.jsonl", "a", buffering=1)

    def log(sev, msg, rid=None, **fields):
        line = json.dumps({"timestamp": now(), "service": name, "severity": sev, "message": msg,
                           "request_id": rid, "fields": fields})
        fh.write(line + "\n"); print(line, flush=True)

    @asynccontextmanager
    async def lifespan(app):
        app.state.client = make_client()
        log("INFO", "service started", port=port)
        if startup: await startup(app)
        yield
        await app.state.client.aclose()

    app = FastAPI(title=name, lifespan=lifespan)
    app.add_middleware(CORSMiddleware, allow_origins=["*"], allow_methods=["*"], allow_headers=["*"])
    app.state.log, app.state.fault, app.state.until = log, FaultConfig(), None

    def active():
        if app.state.until and time.time() > app.state.until:
            app.state.fault, app.state.until = FaultConfig(), None
        return app.state.fault

    @app.exception_handler(Down)
    async def _down(request, exc):
        return err(502, "upstream_unavailable", f"downstream {exc} failed")

    @app.middleware("http")
    async def mw(request: Request, call_next):
        p = request.url.path
        if p in QUIET or p.startswith("/admin"):
            return await call_next(request)
        rid = request.headers.get("x-request-id") or "req_" + secrets.token_hex(8)
        request.state.request_id = rid
        t, resp, f = time.perf_counter(), None, active()
        if f.type == "latency" and f.latency_ms:
            log("ERROR", "injected fault triggered", rid, fault_type="latency")
            await asyncio.sleep(f.latency_ms / 1000)
        elif f.type == "error" and random.random() < f.error_rate:
            log("ERROR", "injected fault triggered", rid, fault_type="error")
            resp = err(500, "internal_error", "injected fault")
        if resp is None:
            try:
                resp = await call_next(request)
            except Exception as e:
                log("ERROR", "unhandled error", rid, error=str(e)[:200])
                resp = err(500, "internal_error", "internal error")
        dt = time.perf_counter() - t; ms = round(dt * 1000)
        route = request.scope.get("route"); tpl = route.path if route else "unmatched"
        REQS.labels(request.method, tpl, str(resp.status_code)).inc()
        DUR.labels(request.method, tpl).observe(dt)
        log("INFO", "request completed", rid, method=request.method, path=tpl, status=resp.status_code, latency_ms=ms)
        if ms > 500:
            log("WARN", "slow request", rid, method=request.method, path=tpl, latency_ms=ms)
        resp.headers["X-Request-ID"] = rid
        return resp

    @app.get("/health")
    async def health():
        return {"service": name, "status": "ok", "version": "1.0.0", "timestamp": now()}

    @app.get("/metrics")
    async def metrics():
        return Response(generate_latest(), media_type=CONTENT_TYPE_LATEST)

    @app.get("/admin/fault")
    async def get_fault():
        return active().model_dump()

    @app.put("/admin/fault")
    async def put_fault(cfg: FaultConfig):
        app.state.fault = cfg
        app.state.until = time.time() + cfg.duration_seconds if cfg.duration_seconds else None
        return cfg.model_dump()

    @app.delete("/admin/fault")
    async def del_fault():
        app.state.fault, app.state.until = FaultConfig(), None
        return app.state.fault.model_dump()

    return app
