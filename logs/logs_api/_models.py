"""Fallback copy of the contract models used here. Real code prefers minidd_contracts.models."""
from __future__ import annotations
from datetime import datetime
from enum import Enum
from typing import Any, Generic, Optional, TypeVar, Literal
from pydantic import BaseModel, ConfigDict, Field

class Base(BaseModel):
    model_config = ConfigDict(extra="forbid")

class Severity(str, Enum):
    DEBUG = "DEBUG"; INFO = "INFO"; WARN = "WARN"; ERROR = "ERROR"

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
