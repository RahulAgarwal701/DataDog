# logs/ (Diya) - Elasticsearch + shipper + Logs API :9002

## Quick demo without anything else (mock mode)
    pip install -r logs/requirements.txt
    MOCK=true python -m logs.logs_api.main      # from repo root
    pytest logs/tests

## Real run
    docker run -d --name es -p 9200:9200 -e discovery.type=single-node -e xpack.security.enabled=false \
      -e ES_JAVA_OPTS="-Xms512m -Xmx512m" docker.elastic.co/elasticsearch/elasticsearch:8.13.4
    LOG_DIR=logs/samples python -m logs.shipper.shipper &     # ships *.jsonl (idempotent, retries if ES is down)
    python -m logs.logs_api.main
    curl 'localhost:9002/api/v1/logs/search?service=payments&severity=ERROR'
    curl 'localhost:9002/api/v1/logs/around?timestamp=2026-10-14T09:30:02Z'
    curl 'localhost:9002/api/v1/logs/stats?start=2026-10-14T00:00:00Z&end=2026-10-15T00:00:00Z'

## Compose snippet (Darsan)
    elasticsearch: {image: docker.elastic.co/elasticsearch/elasticsearch:8.13.4, environment: [discovery.type=single-node, xpack.security.enabled=false, "ES_JAVA_OPTS=-Xms512m -Xmx512m"], ports: ["9200:9200"]}
    log-shipper:   {build: {context: .., dockerfile: logs/Dockerfile}, command: python -m logs.shipper.shipper, environment: [ES_URL=http://elasticsearch:9200, LOG_DIR=/logs], volumes: ["logs:/logs"]}
    logs-api:      {build: {context: .., dockerfile: logs/Dockerfile}, environment: [ES_URL=http://elasticsearch:9200], ports: ["9002:9002"]}

Idempotency: each line's ES `_id` = sha1(line) with `create` op, so re-reads after restart never duplicate.
Count check: `curl localhost:9200/minidd-logs/_count` vs `wc -l logs/*.jsonl`.
