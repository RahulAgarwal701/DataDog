FROM python:3.11-slim
WORKDIR /app
# This host's link to PyPI is slow and drops connections; pip's 15 s default is not enough.
ENV PIP_DEFAULT_TIMEOUT=180 PIP_RETRIES=10 PIP_DISABLE_PIP_VERSION_CHECK=1
# Build context = REPO ROOT (see demo/docker-compose.yml).
COPY darsan/contracts /app/contracts
COPY darsan/platform_api /app/platform_api
COPY darsan/scripts /app/scripts
RUN pip install --no-cache-dir -r platform_api/requirements.txt -e contracts/python
ENV PYTHONUNBUFFERED=1
CMD ["python", "platform_api/main.py"]
