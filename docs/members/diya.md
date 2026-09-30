# Diya — log shipper, Elasticsearch, Logs API

**Folder you own:** `logs/`
**Ports:** 9002 (Logs API), 9200 (Elasticsearch)
**Phase 2/3:** the correlation engine (you own it, and it is the capstone), `logs-api`
retries, `minidd-logs-000001` alias/ILM, API latency benchmarks, sampling for 10k+ rps

## What you built, in one paragraph

The services write one JSON line per log record to a shared volume. The shipper tails those
files, validates every line against the contract's `LogRecord` model, and bulk-indexes into
Elasticsearch with a content hash as the document id, so re-shipping is a no-op. The Logs
API reads Elasticsearch and exposes three things: full-text search, a
"time window around this timestamp" query, and severity counts.

## The three deliverables

**`logs/shipper/shipper.py`**

- installs the index template on boot, retries until Elasticsearch is up
- tails `$LOG_DIR/*.jsonl` on a 1s poll
- validates each line with `LogRecord.model_validate_json`; invalid lines are **skipped
  with a warning, not fatal** — one bad line must not stop the pipeline
- `_id` = SHA-1 of the line, so the same line indexed twice is the same document
- batches into the ES `_bulk` API; raises if any sub-request failed, rather than silently
  losing records

**`logs/elasticsearch/index_template.json`**

- `timestamp` as `date`, `service`/`severity`/`event`/`request_id` as `keyword`
- 1 shard, 0 replicas — deliberate for a single-node demo
- index pattern `minidd-logs*`, so a rollover does not lose the template

**`logs/logs_api/main.py`**

| Route | Purpose |
|---|---|
| `GET /health` | liveness |
| `GET /api/v1/logs/search` | filter by service, severity, free text, time range; paginated |
| `GET /api/v1/logs/around` | ±seconds around a timestamp — **built for the correlation engine** |
| `GET /api/v1/logs/stats` | counts by severity and service over a window |

## Why `/api/v1/logs/around` exists

This is the endpoint to explain. Search answers "what did the logs say". `/around` answers
"what was happening *at this moment*", which is a different and much more useful question.

In Phase 2, when the anomaly detector says "orders' latency breached at 14:32:10", the
correlation engine will call `/around?timestamp=14:32:10&before_seconds=60&after_seconds=30`.
That is the whole reason the endpoint is shaped that way — it was designed for the consumer
that does not exist yet, and it is the seam between your logs work and your correlation
work. If the supervisor asks "what would you do next", this is the honest answer and it is
already built.

## How to demo it

```bash
make -C demo preflight        # confirms logs are arriving
```

Logs tab in the dashboard, or:

```bash
curl -s "localhost:9002/api/v1/logs/search?service=orders&severity=ERROR" | python3 -m json.tool
curl -s "localhost:9002/api/v1/logs/around?timestamp=2026-10-01T14:32:10Z&before_seconds=60&after_seconds=30"
```

Then `make -C demo fault-refuse`, filter to `orders` + `ERROR`, and you get the failure
record. Verified shape, captured live:

```json
{
  "service": "orders",
  "severity": "ERROR",
  "message": "downstream call failed",
  "request_id": "req_65089bc9571f5322",
  "fields": {
    "downstream": "payments",
    "error": "ConnectError: All connection attempts failed -> ConnectionRefusedError: [Errno 111] Connect call failed ('172.19.0.7', 8002)",
    "latency_ms": 2
  }
}
```

Two things to notice, because they will come up. The `message` is generic and human-readable
("downstream call failed"); the diagnostic detail is in `fields.error`. And the `->` in that
string is the **exception chain** — httpx's `ConnectError` wrapping the underlying
`ConnectionRefusedError` — which is the reason the scaffold walks `__cause__` and
`__context__` rather than just calling `str(e)`. Without that walk the log would only ever
say "all connection attempts failed" and the demo would have no proof the failure was a
*refusal* rather than a timeout.

## Known gap in search (found while writing this, not yet fixed)

**Free-text search does not look inside `fields`.** `q` is applied to `message` only, both
in the Elasticsearch query (`{"match": {"message": q}}`) and in the in-memory filter
(`q.lower() in x.message.lower()`). Measured right now:

```
q=downstream      (in message) -> 26,827 hits
q=ECONNREFUSED    (in fields)  ->      0 hits
q=Errno           (in fields)  ->      0 hits
```

So if the supervisor types "ECONNREFUSED" or "Errno 111" into the Logs search box they get
an empty result, even though the data is sitting in the index. The demo is unaffected
because it filters on `service` + `severity=ERROR` and the Logs tab *renders* `fields`, so
the text is visible on screen — but only if you look at the rendered row.

The fix is small: a `multi_match` across `message` and the fields sub-fields (or indexing
`fields.error` as its own mapped field and including it in the query), plus the same change
in the Python-side filter. Until then, do not demonstrate free-text search as a headline
feature, and if the supervisor searches for the error string, tell them it is a known gap
rather than letting them conclude the logs are missing.

## Questions you will actually get

**"How do you avoid duplicate logs?"**

The document `_id` is the SHA-1 of the line. Elasticsearch's `create` operation is an
upsert-by-id, so shipping the same line twice produces one document. This matters because
the shipper tracks a byte offset per file, and a restart, a truncation, or a re-read after
a partial line would otherwise re-index. The second protection is that the offset only
advances past a line once it sees the trailing newline, so a half-written line is never
read. There is also explicit truncation detection: if the file is smaller than the stored
offset, the offset resets to 0.

**"What happens to a malformed log line?"**

It is skipped, logged as a warning, and the offset still advances. The reasoning: a log
shipper that halts on one bad line is a single point of failure for all observability. One
service emitting a malformed record should cost you that record, not the pipeline. It also
means a bug in one service cannot blind the other three.

**"Why is `service` a keyword and not text?"**

Because you aggregate on it. Every service filter and every severity count is an exact
match or a terms aggregation, and `keyword` stores the value unanalysed so it is exact and
usable in aggregations. `text` would tokenise it and break the aggregations. Same reason
`severity` and `request_id` are keywords — `request_id` especially, because the dashboard's
trace-by-request-id is an exact term query.

**"How do you search at scale?"**

Honestly: not yet. Current state is 1 shard, 0 replicas, no ILM, no alias, `match_all` with
a limit. At the demo's 5 rps across 4 services it is a few thousand documents and searches
return in milliseconds. The plan for real volume is index rollover on size or age with
`minidd-logs-000001` and an alias, plus ILM for deletion. Do not claim the current setup
scales — it is a single-node demo configuration and that is a reasonable choice at this
volume.

**"Why Elasticsearch and not a database?"**

Three things it gives for free that this project needs: full-text search over log messages,
time-series shaped aggregation (the severity counts), and an index template that pins the
mapping. A SQL store would need a separate search index to do the text search. The cost is
a JVM in the stack and no real durability, which is acceptable for a demo whose logs are
regenerable by restarting the load generator.

**"Is the log volume realistic?"**

The services log at INFO per request, which is far more verbose than a real system — real
services would sample or log less. The upside is that there is always something to show in
the demo. Real production would add sampling for 10k+ rps, which is on the Phase 3 list.

## Things that will trip you up

- **Shipper lag is up to ~1s**, plus Elasticsearch's own refresh interval, so a log written
  "now" may not be searchable yet. If you inject a fault and immediately search, you may see
  nothing. Wait a couple of seconds.
- **`limit` is capped and paginated.** The dashboard shows the newest 1000 client-side, so
  request tracing only reaches back about 45 seconds at 5 rps. If the supervisor asks for a
  request from two minutes ago, that is why it is not there.
- **The `q` search box only covers `message`.** See the known-gap section above. This is the
  one thing most likely to embarrass you in the demo, because the evidence you want to show
  is in `fields`, not `message`.
- **The dashboard's trace-by-request-id is client-side filtering**, not a server-side query
  by request id. The API supports it; the UI does not use it that way. That is a real gap
  between what the contract offers and what the dashboard consumes, and it is a fair
  criticism if raised.
- **Elasticsearch takes ~20–30s to become writable on first boot.** The shipper waits and
  retries rather than crashing, so the service comes up and catches up. During that window
  the Logs tab is legitimately empty.
- **A restarted Elasticsearch loses nothing** (the volume is not persisted, so it does lose
  the indexed data) but the shipper's offsets are in memory too, so it re-reads from the
  JSONL files. Because `_id` is a content hash, the re-index is idempotent. This is the
  idempotency design paying for itself.

## Your own commands

```bash
# 4 passed -- verified. pytest IS in logs/requirements.txt, so the running
# container can run them; note the path, the Dockerfile copies logs/ to /app/logs.
docker exec minidd-logs-api-1 sh -c 'cd /app/logs && python -m pytest tests/ -q'

curl localhost:9200/_cat/indices/minidd-logs?v
curl localhost:9200/minidd-logs/_mapping
curl -s "localhost:9002/api/v1/logs/stats"
```

Your design notes are in `logs/README.md`. The Phase 2 correlation design is in
`Mini_Datadog_Project_Report.md` (Diya owns "Logs + correlation") and
`01_PLAN_AND_DEMO.md`. The correlation endpoint is already reserved in the contract and
already proxied by the Platform API, so Phase 2 plugs in without touching the UI.

Note: unlike Rahul's and Dev's components, `logs/` has no standalone compose file — it
runs as part of `demo/docker-compose.yml`. Your component was the last to be wired into
the single-stack demo, so it has no isolated-compose path. Worth naming if asked.
