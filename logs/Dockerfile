# Build context = REPO ROOT (see demo/docker-compose.yml).
# Use CMD override for the shipper: python -m logs.shipper.shipper
FROM python:3.11-slim
WORKDIR /app
# This host's link to PyPI is slow and drops connections; pip's 15 s default is not enough.
ENV PIP_DEFAULT_TIMEOUT=180 PIP_RETRIES=10 PIP_DISABLE_PIP_VERSION_CHECK=1
COPY darsan/contracts/python /app/contracts/python
COPY logs /app/logs
RUN pip install --no-cache-dir -r logs/requirements.txt -e contracts/python
ENV PYTHONPATH=/app
CMD ["python", "-m", "logs.logs_api.main"]
