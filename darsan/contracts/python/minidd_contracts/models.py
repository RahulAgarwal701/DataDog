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
    value: Optional[float]

class MetricSeries(Base):
    service: str
    metric: MetricName
    unit: Literal["req/s", "ratio", "ms", "percent", "MB"]
    step_seconds: int
    start: datetime
    end: datetime
    points: list[MetricPoint]

class MetricSnapshot(Base):
    service: str
    timestamp: datetime
    values: dict[MetricName, Optional[float]]

# ---------- logs ----------
class LogRecord(Base):
    id: Optional[str] = None
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
    CONNECT = "CONNECT"; ACCEPT = "ACCEPT"; CLOSE = "CLOSE"; CONNECT_FAILED = "CONNECT_FAILED"

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
    src_service: Optional[str] = None
    dst_service: Optional[str] = None
    pid: int
    process_name: str
    duration_ms: Optional[float] = None
    error: Optional[str] = None

class NetworkEventBatch(Base):
    events: list[NetworkEvent]

class IngestAck(Base):
    accepted: int
    rejected: int = 0

class TopologyNode(Base):
    id: str
    kind: Literal["service", "external"]

class EdgeStatus(str, Enum):
    ACTIVE = "ACTIVE"; STALE = "STALE"

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
    suspected_services: list[str] = Field(default_factory=list)
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
    duration_seconds: Optional[int] = None

class DemoFaultRequest(Base):
    service: str
    fault: FaultConfig

class LoadProfile(Base):
    profile: Literal["baseline", "surge", "stop"]
    rps: Optional[float] = Field(None, gt=0)

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
