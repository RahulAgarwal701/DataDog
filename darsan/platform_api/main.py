"""Platform API :9000 - single entry for the UI. Transparent proxy + /services, /health/all, /demo/fault."""
import asyncio, os
from datetime import datetime, timezone
import httpx, yaml
from fastapi import FastAPI, Request, Response
from fastapi.exceptions import RequestValidationError
from fastapi.middleware.cors import CORSMiddleware
from fastapi.responses import JSONResponse
from minidd_contracts.models import (AllHealthResponse, DemoFaultRequest, ErrorBody, ErrorResponse, FaultConfig,
                                     HealthResponse, ServiceHealth, ServiceInfo)

REGISTRY = os.getenv("REGISTRY_PATH", "contracts/registry.yaml")
SVC_FMT = os.getenv("SERVICE_URL_FMT", "http://localhost:{port}")  # Compose: http://{name}:{port}
U = lambda k, d: os.getenv(k, d)
PROXY = [  # (path prefix, upstream)
    ("/api/v1/metrics", U("METRICS_URL", "http://localhost:9001")),
    ("/api/v1/logs", U("LOGS_URL", "http://localhost:9002")),
    ("/api/v1/topology", U("TOPOLOGY_URL", "http://localhost:9003")),
    ("/api/v1/network-events", U("TOPOLOGY_URL", "http://localhost:9003")),
    ("/api/v1/anomalies", U("ANOMALY_URL", "http://localhost:9004")),
    ("/api/v1/incidents", U("CORRELATION_URL", "http://localhost:9005")),
]
SERVICES = [ServiceInfo(**s) for s in yaml.safe_load(open(REGISTRY))["services"]]
url_of = lambda s: SVC_FMT.format(name=s.name, port=s.port)

app = FastAPI(title="platform-api")
app.add_middleware(CORSMiddleware, allow_origins=["*"], allow_methods=["*"], allow_headers=["*"])

def err(status, code, msg):
    return JSONResponse(status_code=status, content=ErrorResponse(error=ErrorBody(code=code, message=msg)).model_dump(mode="json"))

@app.exception_handler(RequestValidationError)
async def _v(_, e): return err(400, "invalid_argument", str(e.errors()[0]["msg"]))

@app.exception_handler(Exception)
async def _e(_, e): return err(500, "internal_error", str(e))

@app.get("/health", response_model=HealthResponse)
async def health(): return HealthResponse(service="platform-api", status="ok", timestamp=datetime.now(timezone.utc))

@app.get("/api/v1/services", response_model=list[ServiceInfo])
async def services(): return SERVICES

async def _health(c, s):
    try:
        r = await c.get(url_of(s) + "/health", timeout=3)
        return ServiceHealth(service=s.name, reachable=True, health=HealthResponse(**r.json()))
    except Exception:
        return ServiceHealth(service=s.name, reachable=False)

@app.get("/api/v1/health/all", response_model=AllHealthResponse)
async def health_all():
    async with httpx.AsyncClient() as c:
        return AllHealthResponse(services=list(await asyncio.gather(*[_health(c, s) for s in SERVICES])))

@app.post("/api/v1/demo/fault", response_model=FaultConfig)
async def demo_fault(req: DemoFaultRequest):
    s = next((x for x in SERVICES if x.name == req.service and x.kind == "service"), None)
    if not s: return err(404, "not_found", f"unknown service {req.service}")
    try:
        async with httpx.AsyncClient() as c:
            r = await c.put(url_of(s) + "/admin/fault", json=req.fault.model_dump(mode="json"), timeout=5)
        return Response(r.content, r.status_code, media_type="application/json")
    except httpx.HTTPError as e:
        return err(503, "upstream_unavailable", f"{req.service} unreachable: {e}")

@app.api_route("/api/v1/{path:path}", methods=["GET", "POST", "PUT", "DELETE"])
async def proxy(path: str, request: Request):
    full = "/api/v1/" + path
    up = next((u for p, u in PROXY if full.startswith(p)), None)
    if not up: return err(404, "not_found", "no such endpoint")
    try:
        async with httpx.AsyncClient() as c:
            r = await c.request(request.method, up + full, params=request.query_params, content=await request.body(),
                                headers={"content-type": request.headers.get("content-type", "application/json")}, timeout=15)
        return Response(r.content, r.status_code, media_type=r.headers.get("content-type", "application/json"))
    except httpx.HTTPError as e:
        return err(503, "upstream_unavailable", f"upstream unreachable: {e}")

if __name__ == "__main__":
    import uvicorn
    uvicorn.run(app, host="0.0.0.0", port=int(os.getenv("PORT", "9000")))
