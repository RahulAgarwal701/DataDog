# Build context = REPO ROOT (see demo/docker-compose.yml).
# Network-event simulator: only needed when the real eBPF agent (linux/ebpf) cannot run.
FROM python:3.11-slim
WORKDIR /app
# This host's link to PyPI is slow and drops connections; pip's 15 s default is not enough.
ENV PIP_DEFAULT_TIMEOUT=180 PIP_RETRIES=10 PIP_DISABLE_PIP_VERSION_CHECK=1
ENV PYTHONUNBUFFERED=1
COPY darsan/contracts/ /app/contracts/
RUN pip install --no-cache-dir "pydantic>=2.6" -e /app/contracts/python
COPY demo/netmon_sim.py /app/netmon_sim.py
ENV REGISTRY_PATH=/app/contracts/registry.yaml
ENTRYPOINT ["python", "/app/netmon_sim.py"]
