# Local mirror of contract DTOs (swap for minidd_contracts.models once v1.0.0 is published)
from typing import Literal, Optional
from pydantic import BaseModel, ConfigDict, Field
class Base(BaseModel):
    model_config = ConfigDict(extra="forbid")
class FaultConfig(Base):
    type: Literal["none", "latency", "error"] = "none"
    latency_ms: int = Field(0, ge=0)
    error_rate: float = Field(0.0, ge=0, le=1)
    duration_seconds: Optional[int] = None
class LoadProfile(Base):
    profile: Literal["baseline", "surge", "stop"]
    rps: Optional[float] = Field(None, gt=0)
class OrderRequest(Base):
    customer_id: str
    item_id: str
    quantity: int = Field(ge=1)
    amount: float = Field(gt=0)
class ChargeRequest(Base):
    order_id: str
    amount: float = Field(gt=0)
    item_id: str
    quantity: int = Field(ge=1)
class ReserveRequest(Base):
    order_id: str
    item_id: str
    quantity: int = Field(ge=1)
