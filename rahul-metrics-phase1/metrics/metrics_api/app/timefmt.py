from datetime import datetime, timezone


def utcnow() -> datetime:
    return datetime.now(timezone.utc)


def ensure_utc(dt: datetime) -> datetime:
    """Naive datetimes are assumed to already be UTC."""
    return dt.replace(tzinfo=timezone.utc) if dt.tzinfo is None else dt.astimezone(timezone.utc)


def fmt_ts(dt: datetime) -> str:
    """Contract format: UTC ISO-8601, millisecond precision, trailing Z."""
    dt = ensure_utc(dt)
    return dt.strftime("%Y-%m-%dT%H:%M:%S.") + f"{dt.microsecond // 1000:03d}Z"


def from_ms(ms: int) -> datetime:
    return datetime.fromtimestamp(ms / 1000.0, tz=timezone.utc)


def to_ms(dt: datetime) -> int:
    return int(round(ensure_utc(dt).timestamp() * 1000))
