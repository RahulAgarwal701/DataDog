import os
from dataclasses import dataclass, field

DEFAULT_SERVICES = ["api-gateway", "orders", "payments", "inventory"]


def _bool(name: str, default: bool) -> bool:
    return os.getenv(name, str(default)).strip().lower() in ("1", "true", "yes", "on")


def load_services(registry_path: str | None) -> list[str]:
    """Monitored services = registry entries with kind=service. Falls back to the contract list."""
    paths = [registry_path] if registry_path else []
    paths += ["contracts/registry.yaml", "/app/contracts/registry.yaml", "../../contracts/registry.yaml"]
    for p in paths:
        if p and os.path.exists(p):
            try:
                import yaml
                with open(p) as f:
                    data = yaml.safe_load(f)
                names = [s["name"] for s in data["services"] if s.get("kind") == "service"]
                if names:
                    return names
            except Exception:
                pass
    return list(DEFAULT_SERVICES)


@dataclass
class Settings:
    port: int = field(default_factory=lambda: int(os.getenv("PORT", "9001")))
    log_level: str = field(default_factory=lambda: os.getenv("LOG_LEVEL", "INFO"))
    prometheus_url: str = field(default_factory=lambda: os.getenv("PROMETHEUS_URL", "http://localhost:9090"))
    mock: bool = field(default_factory=lambda: _bool("MOCK", False))
    # In MOCK mode, payments latency spikes for 30s out of every 180s so the UI has something to show.
    mock_fault_cycle: bool = field(default_factory=lambda: _bool("MOCK_FAULT_CYCLE", True))
    prom_timeout: float = field(default_factory=lambda: float(os.getenv("PROM_TIMEOUT_SECONDS", "5")))
    registry_path: str | None = field(default_factory=lambda: os.getenv("REGISTRY_PATH"))
    services: list[str] = field(default_factory=list)

    def __post_init__(self):
        if not self.services:
            self.services = load_services(self.registry_path)
