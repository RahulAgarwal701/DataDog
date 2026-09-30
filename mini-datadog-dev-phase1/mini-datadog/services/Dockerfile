# Build context = REPO ROOT (see demo/docker-compose.yml).
# One image for all five modules; MODULE selects which one to run.
FROM python:3.11-slim
WORKDIR /app
# This host's link to PyPI is slow and drops connections; pip's 15 s default is not enough.
ENV PIP_DEFAULT_TIMEOUT=180 PIP_RETRIES=10 PIP_DISABLE_PIP_VERSION_CHECK=1
COPY darsan/contracts/ /app/contracts/
COPY mini-datadog-dev-phase1/mini-datadog/services/requirements.txt requirements.txt
# The real contract package is installed too, so tests can prove the local model
# mirrors in services/common/models.py still match it (see `make -C demo check`).
RUN pip install --no-cache-dir -r requirements.txt -e /app/contracts/python
COPY mini-datadog-dev-phase1/mini-datadog/services/ /app/services/
ENV LOG_DIR=/logs PYTHONUNBUFFERED=1
CMD ["sh", "-c", "uvicorn services.${MODULE}.main:app --host 0.0.0.0 --port ${PORT}"]
