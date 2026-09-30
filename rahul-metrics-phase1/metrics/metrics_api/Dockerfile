# Build context = REPO ROOT (see demo/docker-compose.yml).
FROM python:3.11-slim
WORKDIR /app
# This host's link to PyPI is slow and drops connections; pip's 15 s default is not enough.
ENV PIP_DEFAULT_TIMEOUT=180 PIP_RETRIES=10 PIP_DISABLE_PIP_VERSION_CHECK=1
ENV PYTHONUNBUFFERED=1 PORT=9001
COPY darsan/contracts/ /app/contracts/
COPY rahul-metrics-phase1/metrics/metrics_api/requirements.txt /app/requirements.txt
RUN pip install --no-cache-dir -r /app/requirements.txt -e /app/contracts/python
COPY rahul-metrics-phase1/metrics/metrics_api/app /app/app
EXPOSE 9001
CMD ["sh", "-c", "uvicorn app.main:app --host 0.0.0.0 --port ${PORT}"]
