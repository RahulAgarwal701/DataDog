# Mini Datadog: Shared Contracts v1.0.0 (FROZEN)

Every member and every coding agent gets this file. **This is the only thing modules may rely on about each other.** Do not invent fields, endpoints, ports, or names. If something is missing or ambiguous, propose a contract change (section 10) instead of improvising.

## 1. Global conventions

- **JSON only.** Field names `snake_case`. Enum values exactly as written.
- **Timestamps:** timezone-aware **UTC**, ISO-8601, millisecond precision, trailing `Z` (e.g. `2026-10-14T09:30:01.412Z`). Query params `start` / `end` use the same format. Never epoch numbers, never local time.
- **Service names** are lowercase-hyphen and must exist in `registry.yaml`: `api-gateway`, `orders`, `payments`, `inventory` (plus external `loadgen`).
- **IDs:** strings. `event_id`, `anomaly_id`, `incident_id` are UUID4 strings. `request_id` is `req_` + 16 hex chars.
- **Units are fixed** per metric (section 4). Never convert in a consumer.
- **Errors:** every non-2xx response body is `ErrorResponse` (section 6). Codes: `invalid_argument` (400), `not_found` (404), `upstream_unavailable` (503), `internal_error` (500).
- **Every backend exposes** `GET /health` → `HealthResponse`.
- **Pagination:** list endpoints take `limit` (default 100, max 1000) and `offset` (default 0) and return `Page`.
- **CORS:** all backends allow `*` in dev.
- **Config:** env vars only (`PORT`, `LOG_LEVEL`, upstream URLs); provide `.env.example`. Defaults must work for localhost; Compose sets service DNS names.
- **Python packages:** import DTOs from `minidd_contracts.models`. **TypeScript:** import from `contracts/ts/types.ts` (generated; never hand-write DTO types).

## 2. Service registry (`contracts/registry.yaml`)

```yaml
services:
  - {name: api-gateway, port: 8000, kind: service,  description: "Public entry point",   depends_on: [orders]}
  - {name: orders,      port: 8001, kind: service,  description: "Order management",     depends_on: [payments]}
  - {name: payments,    port: 8002, kind: service,  description: "Payment processing",   depends_on: [inventory]}
  - {name: inventory,   port: 8003, kind: service,  description: "Stock reservation",    depends_on: []}
  - {name: loadgen,     port: 8010, kind: external, description: "Traffic generator",    depends_on: [api-gateway]}
```

## 3. Ports and hostnames (Compose service name = hostname)

| Component | Host:port | Owner |
|---|---|---|
| api-gateway / orders / payments / inventory | :8000 / :8001 / :8002 / :8003 | Dev |
| loadgen | :8010 | Dev |
| platform-api (single entry for the UI) | :9000 | Darsan |
| dashboard | :3000 | Darsan |
| metrics-api | :9001 | Rahul |
| prometheus | :9090 | Rahul |
| logs-api | :9002 | Diya |
| elasticsearch | :9200 | Diya |
| topology-api | :9003 | Brajesh |
| anomaly-detector (Phase 2) | :9004 | Dev |
| correlation (Phase 2) | :9005 | Diya |
| ebpf-agent | runs on the host (not a network service) | Brajesh |
| log-shipper | no port | Diya |

Downstream URLs via env: `ORDERS_URL`, `PAYMENTS_URL`, `INVENTORY_URL`, `PROMETHEUS_URL`, `ES_URL`, `TOPOLOGY_URL`, `LOGS_URL`, `METRICS_URL`, `ANOMALY_URL`, `CORRELATION_URL`.

## 4. Metrics contract

### 4.1 What every dummy service exposes on `GET /metrics` (Prometheus text format)

- `http_requests_total{method, path, status}`: counter. `path` is the **route template** (e.g. `/charge`), never the raw URL.
- `http_request_duration_seconds{method, path}`: histogram, buckets `[0.005,0.01,0.025,0.05,0.1,0.25,0.5,1,2.5,5]`.
- Default process metrics: `process_cpu_seconds_total`, `process_resident_memory_bytes`.
- Prometheus attaches the `service` label from its scrape config; services do **not** set it. Scrape interval **5s**.

### 4.2 Canonical metrics (Metrics API output) and the exact PromQL (`$S` = service)

| `metric` | unit | PromQL |
|---|---|---|
| `request_rate` | `req/s` | `sum(rate(http_requests_total{service="$S"}[30s]))` |
| `error_rate` | `ratio` (0..1) | `sum(rate(http_requests_total{service="$S",status=~"5.."}[30s])) / clamp_min(sum(rate(http_requests_total{service="$S"}[30s])), 0.0001)` |
| `latency_p95_ms` | `ms` | `1000 * histogram_quantile(0.95, sum by (le) (rate(http_request_duration_seconds_bucket{service="$S"}[30s])))` |
| `cpu_percent` | `percent` (of one core, 0..100+) | `100 * rate(process_cpu_seconds_total{service="$S"}[30s])` |
| `memory_mb` | `MB` | `process_resident_memory_bytes{service="$S"} / 1048576` |

## 5. Log contract

Dummy services write **one JSON object per line** to `$LOG_DIR/<service>.jsonl` (default `./logs`; in Compose a shared volume mounted at `/logs`) and also to stdout. Each line is exactly a `LogRecord` (without `id`).

```json
{"timestamp":"2026-10-14T09:30:01.412Z","service":"payments","severity":"ERROR","message":"injected fault triggered","request_id":"req_8f3a9c21d0b47e55","fields":{"path":"/charge","status":500,"latency_ms":812}}
```

Required log events from every service (fixed `message` strings so correlation can rely on them):

| Severity | `message` | `fields` |
|---|---|---|
| INFO | `request completed` | `method, path, status, latency_ms` |
| WARN | `slow request` (when latency_ms > 500) | `method, path, latency_ms` |
| ERROR | `downstream call failed` | `downstream, error, latency_ms` |
| ERROR | `injected fault triggered` | `fault_type` |
| INFO | `service started` | `port` |

## 6. DTOs: `contracts/python/minidd_contracts/models.py` (SOURCE OF TRUTH)

```python
from __future__ import annotations
from datetime import datetime
from enum import Enum
from typing import Any, Generic, Literal, Optional, TypeVar
from pydantic import BaseModel, ConfigDict, Field

class Base(BaseModel):
    model_config = ConfigDict(extra="forbid")   # unknown fields = contract violation

# ---------- shared ----------
class Severity(str, Enum):
    DEBUG = "DEBUG"; INFO = "INFO"; WARN = "WARN"; ERROR = "ERROR"

class AlertSeverity(str, Enum):
    LOW = "LOW"; MEDIUM = "MEDIUM"; HIGH = "HIGH"; CRITICAL = "CRITICAL"

class MetricName(str, Enum):
    request_rate = "request_rate"
    latency_p95_ms = "latency_p95_ms"
    error_rate = "error_rate"
    cpu_percent = "cpu_percent"
    memory_mb = "memory_mb"

class HealthResponse(Base):
    service: str
    status: Literal["ok", "degraded"]
    version: str = "1.0.0"
    timestamp: datetime

class ErrorBody(Base):
    code: Literal["invalid_argument", "not_found", "upstream_unavailable", "internal_error"]
    message: str
    details: Optional[dict[str, Any]] = None

class ErrorResponse(Base):
    error: ErrorBody

T = TypeVar("T")
class Page(Base, Generic[T]):
    items: list[T]
    total: int
    limit: int
    offset: int

# ---------- metrics ----------
class MetricPoint(Base):
    ts: datetime
    value: Optional[float]            # null = gap (no data)

class MetricSeries(Base):
    service: str
    metric: MetricName
    unit: Literal["req/s", "ratio", "ms", "percent", "MB"]
    step_seconds: int
    start: datetime
    end: datetime
    points: list[MetricPoint]

class MetricSnapshot(Base):           # latest values for one service
    service: str
    timestamp: datetime
    values: dict[MetricName, Optional[float]]

# ---------- logs ----------
class LogRecord(Base):
    id: Optional[str] = None          # assigned by Logs API/Elasticsearch
    timestamp: datetime
    service: str
    severity: Severity
    message: str
    request_id: Optional[str] = None
    fields: dict[str, Any] = Field(default_factory=dict)

class LogStats(Base):
    start: datetime
    end: datetime
    by_service: dict[str, int]
    by_severity: dict[Severity, int]

# ---------- network / topology ----------
class NetworkEventType(str, Enum):
    CONNECT = "CONNECT"               # successful outbound connection (client side)
    ACCEPT = "ACCEPT"                 # inbound connection accepted (server side)
    CLOSE = "CLOSE"                   # connection closed (duration_ms set)
    CONNECT_FAILED = "CONNECT_FAILED" # refused / timed out (error set)

class NetworkEvent(Base):
    event_id: str
    timestamp: datetime
    host: str
    event_type: NetworkEventType
    protocol: Literal["tcp"] = "tcp"
    src_ip: str
    src_port: int
    dst_ip: str
    dst_port: int
    src_service: Optional[str] = None # resolved by agent; null if unresolved
    dst_service: Optional[str] = None
    pid: int
    process_name: str
    duration_ms: Optional[float] = None   # CLOSE only
    error: Optional[str] = None           # CONNECT_FAILED only: "ECONNREFUSED" | "ETIMEDOUT" | "ECONNRESET"

class NetworkEventBatch(Base):
    events: list[NetworkEvent]

class IngestAck(Base):
    accepted: int
    rejected: int = 0

class TopologyNode(Base):
    id: str
    kind: Literal["service", "external"]

class EdgeStatus(str, Enum):
    ACTIVE = "ACTIVE"                 # seen in the last 60 s
    STALE = "STALE"

class TopologyEdge(Base):
    source: str
    target: str
    first_seen: datetime
    last_seen: datetime
    connection_count: int
    failed_count: int
    status: EdgeStatus

class TopologyGraph(Base):
    generated_at: datetime
    window_seconds: int
    nodes: list[TopologyNode]
    edges: list[TopologyEdge]

# ---------- anomalies & incidents (Phase 2, frozen now) ----------
class Anomaly(Base):
    anomaly_id: str
    service: str
    metric: MetricName
    detected_at: datetime
    observed_value: float
    baseline_mean: float
    baseline_std: float
    z_score: float
    threshold: float
    direction: Literal["above", "below"]
    severity: AlertSeverity
    method: str = "zscore"
    baseline_window_seconds: int
    evaluation_window_seconds: int

class LogEvidence(Base):
    log: LogRecord
    score: float = Field(ge=0, le=1)
    reason: str

class NetworkEvidence(Base):
    event: NetworkEvent
    score: float = Field(ge=0, le=1)
    reason: str

class IncidentEvidence(Base):
    logs: list[LogEvidence] = Field(default_factory=list)
    network: list[NetworkEvidence] = Field(default_factory=list)
    topology_edges: list[TopologyEdge] = Field(default_factory=list)

class IncidentStatus(str, Enum):
    OPEN = "OPEN"; RESOLVED = "RESOLVED"

class Incident(Base):
    incident_id: str
    status: IncidentStatus
    severity: AlertSeverity
    title: str
    service: str
    metric: MetricName
    created_at: datetime
    updated_at: datetime
    anomaly: Anomaly
    evidence: IncidentEvidence
    suspected_services: list[str] = Field(default_factory=list)  # ranked, most likely first
    summary: Optional[str] = None

# ---------- platform / dummy services ----------
class ServiceInfo(Base):
    name: str
    port: int
    kind: Literal["service", "external"]
    description: str
    depends_on: list[str]

class ServiceHealth(Base):
    service: str
    reachable: bool
    health: Optional[HealthResponse] = None

class AllHealthResponse(Base):
    services: list[ServiceHealth]

class FaultConfig(Base):
    type: Literal["none", "latency", "error"] = "none"
    latency_ms: int = Field(0, ge=0)
    error_rate: float = Field(0.0, ge=0, le=1)
    duration_seconds: Optional[int] = None    # auto-clear after N seconds; null = until cleared

class DemoFaultRequest(Base):
    service: str
    fault: FaultConfig

class LoadProfile(Base):
    profile: Literal["baseline", "surge", "stop"]
    rps: Optional[float] = Field(None, gt=0)  # null = profile default (baseline 5, surge 20)

class OrderRequest(Base):
    customer_id: str
    item_id: str
    quantity: int = Field(ge=1)
    amount: float = Field(gt=0)

class OrderResponse(Base):
    order_id: str
    status: Literal["confirmed", "failed"]
    request_id: str

class ChargeRequest(Base):
    order_id: str
    amount: float = Field(gt=0)
    item_id: str
    quantity: int = Field(ge=1)

class ChargeResponse(Base):
    payment_id: str
    status: Literal["captured", "declined"]

class ReserveRequest(Base):
    order_id: str
    item_id: str
    quantity: int = Field(ge=1)

class ReserveResponse(Base):
    reservation_id: str
    status: Literal["reserved", "out_of_stock"]
```

`Page[LogRecord]`, `Page[NetworkEvent]`, `Page[Anomaly]`, `Page[Incident]` are the paged responses.

## 7. Endpoints (all paths are final)

### 7.1 Dummy services (Dev): each on its own port
| Method & path | Body → Response |
|---|---|
| `GET /health`, `GET /metrics` | `HealthResponse`, Prometheus text |
| `GET / PUT / DELETE /admin/fault` | `FaultConfig` (PUT sets, DELETE clears → `none`) |
| `POST /orders` (**api-gateway** and **orders**) | `OrderRequest` → `OrderResponse` (201; 502 `ErrorResponse` if downstream fails) |
| `POST /charge` (**payments**) | `ChargeRequest` → `ChargeResponse` |
| `POST /reserve` (**inventory**) | `ReserveRequest` → `ReserveResponse` |

Call chain: `api-gateway POST /orders → orders POST /orders → payments POST /charge → inventory POST /reserve`. Propagate `X-Request-ID` on every hop. Outgoing HTTP calls disable keep-alive by default (`HTTP_KEEPALIVE=false`) so each call opens a new TCP connection, which is what makes eBPF events visible. Each process sets env `SERVICE_NAME=<name>` (used by the eBPF agent to identify the connecting process). One uvicorn worker per service.

Loadgen (:8010): `GET /health`; `GET /profile` and `POST /profile` (`LoadProfile`).

### 7.2 Metrics API (Rahul) :9001
| Endpoint | Response |
|---|---|
| `GET /api/v1/metrics/query?service=&metric=&start=&end=&step_seconds=` | `MetricSeries` (defaults: last 15 min, step 5; max 2000 points → else 400) |
| `GET /api/v1/metrics/latest?service=` (service optional = all) | `list[MetricSnapshot]` |

### 7.3 Logs API (Diya) :9002
| Endpoint | Response |
|---|---|
| `GET /api/v1/logs/search?service=&severity=&q=&start=&end=&limit=&offset=&order=desc` (`service`, `severity` repeatable; `q` = full-text on message) | `Page[LogRecord]` |
| `GET /api/v1/logs/around?timestamp=&before_seconds=60&after_seconds=30&service=&min_severity=INFO` | `Page[LogRecord]`, ascending by time |
| `GET /api/v1/logs/stats?start=&end=` | `LogStats` |

### 7.4 Topology API (Brajesh) :9003
| Endpoint | Response |
|---|---|
| `POST /api/v1/network-events` (`NetworkEventBatch`, ≤ 500 events) | 202 `IngestAck` |
| `GET /api/v1/network-events?service=&event_type=&start=&end=&limit=&offset=` (`service` matches src or dst) | `Page[NetworkEvent]`, newest first |
| `GET /api/v1/topology?window_seconds=300` | `TopologyGraph` |

Topology rule: an edge `A→B` is built from `CONNECT` and `CONNECT_FAILED` events where `src_service=A` and `dst_service=B` (`failed_count` counts the latter). Nodes = all registry entries seen in edges plus every registry `service` (so isolated services still appear).

### 7.5 Anomaly detector (Dev) :9004 (Phase 2)
`GET /api/v1/anomalies?service=&start=&end=&limit=&offset=` → `Page[Anomaly]`; `GET /api/v1/anomalies/{anomaly_id}` → `Anomaly`. On detection it pushes `POST {CORRELATION_URL}/api/v1/correlation/anomalies` (`Anomaly`) → 202.

### 7.6 Correlation (Diya) :9005 (Phase 2)
`GET /api/v1/incidents?status=&service=&start=&end=&limit=&offset=` → `Page[Incident]`; `GET /api/v1/incidents/{incident_id}` → `Incident`.

### 7.7 Platform API (Darsan) :9000
Transparent proxy (same paths, status codes and bodies): `/api/v1/metrics/*` → 9001, `/api/v1/logs/*` → 9002, `/api/v1/topology*` and `/api/v1/network-events*` → 9003, `/api/v1/anomalies*` → 9004, `/api/v1/incidents*` → 9005. If an upstream is unreachable, return 503 `ErrorResponse`. Its own endpoints: `GET /api/v1/services` → `list[ServiceInfo]`, `GET /api/v1/health/all` → `AllHealthResponse`, `POST /api/v1/demo/fault` (`DemoFaultRequest`) → forwards to the service's `PUT /admin/fault`, returns the resulting `FaultConfig`.

## 8. Fixtures (`contracts/fixtures/*.json`; Darsan generates the full set, and every service's mock mode and tests use them)

```json
// MetricSeries
{"service":"payments","metric":"latency_p95_ms","unit":"ms","step_seconds":5,
 "start":"2026-10-14T09:30:00.000Z","end":"2026-10-14T09:30:10.000Z",
 "points":[{"ts":"2026-10-14T09:30:00.000Z","value":54.2},{"ts":"2026-10-14T09:30:05.000Z","value":null}]}

// NetworkEvent (success, then failure)
{"event_id":"6f1c2a7e-3b0d-4d7e-9a55-0c2f1d8e4b10","timestamp":"2026-10-14T09:30:01.410Z","host":"demo-vm",
 "event_type":"CONNECT","protocol":"tcp","src_ip":"172.18.0.4","src_port":51522,"dst_ip":"172.18.0.5","dst_port":8002,
 "src_service":"orders","dst_service":"payments","pid":4121,"process_name":"python","duration_ms":null,"error":null}
{"event_id":"a91b0c33-52de-4a88-b7e1-99d3c6a2f0aa","timestamp":"2026-10-14T09:31:14.020Z","host":"demo-vm",
 "event_type":"CONNECT_FAILED","protocol":"tcp","src_ip":"172.18.0.4","src_port":51610,"dst_ip":"172.18.0.5","dst_port":8002,
 "src_service":"orders","dst_service":"payments","pid":4121,"process_name":"python","duration_ms":null,"error":"ECONNREFUSED"}

// TopologyGraph
{"generated_at":"2026-10-14T09:30:05.000Z","window_seconds":300,
 "nodes":[{"id":"loadgen","kind":"external"},{"id":"api-gateway","kind":"service"},{"id":"orders","kind":"service"},{"id":"payments","kind":"service"},{"id":"inventory","kind":"service"}],
 "edges":[{"source":"orders","target":"payments","first_seen":"2026-10-14T09:25:01.000Z","last_seen":"2026-10-14T09:30:04.000Z","connection_count":1520,"failed_count":0,"status":"ACTIVE"}]}

// Anomaly
{"anomaly_id":"0d9f7c1a-8e2b-4f6a-b3c4-1a2b3c4d5e6f","service":"payments","metric":"latency_p95_ms","detected_at":"2026-10-14T09:30:45.000Z",
 "observed_value":910.4,"baseline_mean":52.1,"baseline_std":6.3,"z_score":136.2,"threshold":3.0,"direction":"above","severity":"CRITICAL",
 "method":"zscore","baseline_window_seconds":300,"evaluation_window_seconds":30}
```

An `Incident` fixture is an `Anomaly` + `evidence` (`logs` with `LogEvidence`, `network` with `NetworkEvidence`, `topology_edges`) + `suspected_services: ["payments"]`. Darsan includes the full one.

## 9. Repository layout and workflow

```
mini-datadog/
├── contracts/            # Darsan (changes need all approvals)
│   ├── python/minidd_contracts/{__init__,models}.py
│   ├── jsonschema/  ts/types.ts     # GENERATED by scripts/gen_contracts.sh
│   ├── fixtures/  registry.yaml  CONTRACT_VERSION
├── services/             # Dev: common/ api_gateway/ orders/ payments/ inventory/ loadgen/
├── scripts/faults/       # Dev
├── metrics/              # Rahul: prometheus/ metrics_api/
├── anomaly/              # Dev (Phase 2)
├── ebpf/  topology/      # Brajesh: agent/  topology_api/
├── logs/                 # Diya: elasticsearch/ shipper/ logs_api/
├── correlation/          # Diya (Phase 2)
├── platform_api/ dashboard/ deploy/ scripts/   # Darsan
└── docs/                 # everyone adds their own doc, e.g. docs/metrics.md
```

- Each backend has its own `Dockerfile` (build context = repo root so it can `COPY contracts/`), `README.md` (run + test), `.env.example`, and `tests/test_contract.py`, which loads fixtures and asserts its endpoints' responses validate against the pydantic models.
- Every service supports `MOCK=true`: serves fixture-shaped data without its real dependencies, so others can integrate early.
- Git: branches `feat/<name>/<topic>`, small PRs into `main`, conventional commits (`feat(metrics): ...`). Commit history is Phase 1 evidence, so commit often and honestly.

## 10. Change policy

1. The contract is frozen at v1.0.0 after checkpoint C0.
2. Need a change? Open an issue labelled `contract-change` describing the field/endpoint and why. One PR modifies `contracts/` only; **all 5 members approve**; Darsan regenerates TS/JSON Schema and bumps `CONTRACT_VERSION`.
3. Additive, optional fields are allowed with a heads-up. Renames or removals after Phase 1 require agreement from all 5.
4. If an agent thinks the contract is wrong, it **stops and reports** rather than working around it.
