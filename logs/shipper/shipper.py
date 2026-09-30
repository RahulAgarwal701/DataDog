"""Log shipper: tails $LOG_DIR/*.jsonl -> validates as LogRecord -> idempotent bulk index into ES."""
import glob, hashlib, json, logging, os, time
import httpx
try:
    from minidd_contracts.models import LogRecord
except ImportError:
    from logs.logs_api._models import LogRecord

ES_URL = os.getenv("ES_URL", "http://localhost:9200")
LOG_DIR = os.getenv("LOG_DIR", "./logs")
INDEX = os.getenv("ES_INDEX", "minidd-logs")
TEMPLATE = os.getenv("ES_TEMPLATE", os.path.join(os.path.dirname(__file__), "..", "elasticsearch", "index_template.json"))
POLL = float(os.getenv("POLL_SECONDS", "1"))
logging.basicConfig(level=os.getenv("LOG_LEVEL", "INFO"), format="%(asctime)s shipper %(levelname)s %(message)s")
log = logging.getLogger("shipper")


def wait_and_setup(c):
    while True:
        try:
            c.put(f"{ES_URL}/_index_template/minidd-logs", json=json.load(open(TEMPLATE))).raise_for_status()
            log.info("ES ready, template installed")
            return
        except Exception as e:
            log.warning("waiting for ES: %s", e); time.sleep(3)


def read_new(path, offset):
    """Return (records, new_offset) for complete lines after offset."""
    if os.path.getsize(path) < offset:
        offset = 0  # truncated/rotated
    out = []
    with open(path, "rb") as f:
        f.seek(offset)
        for raw in f:
            if not raw.endswith(b"\n"):
                break  # partial line, wait for the rest
            offset += len(raw)
            line = raw.decode().strip()
            if not line:
                continue
            try:
                rec = LogRecord.model_validate_json(line)
                out.append((hashlib.sha1(line.encode()).hexdigest(), rec))
            except Exception as e:
                log.warning("skip invalid line in %s: %s", path, str(e)[:100])
    return out, offset


def bulk(c, docs):
    body = ""
    for _id, rec in docs:
        body += json.dumps({"create": {"_index": INDEX, "_id": _id}}) + "\n"
        body += rec.model_dump_json(exclude={"id"}) + "\n"
    r = c.post(f"{ES_URL}/_bulk", content=body, headers={"Content-Type": "application/x-ndjson"})
    r.raise_for_status()
    bad = [i for i in r.json()["items"] if i["create"]["status"] not in (201, 409)]  # 409 = already indexed
    if bad:
        raise RuntimeError(f"{len(bad)} bulk failures, e.g. {bad[0]}")


def main():
    offsets = {}
    with httpx.Client(timeout=30) as c:
        wait_and_setup(c)
        while True:
            try:
                for path in glob.glob(os.path.join(LOG_DIR, "*.jsonl")):
                    docs, new_off = read_new(path, offsets.get(path, 0))
                    for i in range(0, len(docs), 500):
                        bulk(c, docs[i:i + 500])
                    if docs:
                        log.info("shipped %d from %s", len(docs), os.path.basename(path))
                    offsets[path] = new_off
            except Exception as e:  # ES down etc: offsets unchanged, retry (ids make it idempotent)
                log.warning("ship failed, retrying: %s", e); time.sleep(3)
            time.sleep(POLL)


if __name__ == "__main__":
    main()
