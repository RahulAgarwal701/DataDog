"""STAND-IN: verbatim subset of contracts v1.0.0 section 6 (only what metrics-api uses).
The real source of truth is Darsan's contracts/python/minidd_contracts/models.py. Delete this dir when that is installable."""
from __future__ import annotations
from datetime import datetime
from enum import Enum
from typing import Any, Literal, Optional
from pydantic import BaseModel, ConfigDict


class Base(BaseModel):
    model_config = ConfigDict(extra="forbid")


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
