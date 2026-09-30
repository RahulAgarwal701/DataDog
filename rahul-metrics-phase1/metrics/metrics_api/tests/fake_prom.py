"""A fake Prometheus HTTP server for tests (stdlib only)."""
import json
import threading
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from urllib.parse import parse_qs, urlparse


class FakeProm:
    def __init__(self, range_fn=None, instant_fn=None):
        self.range_fn = range_fn or (lambda q, s, e, st: [])
        self.instant_fn = instant_fn or (lambda q: None)
        self.queries: list[str] = []
        outer = self

        class H(BaseHTTPRequestHandler):
            def log_message(self, *a):
                pass

            def _send(self, obj, code=200):
                b = json.dumps(obj).encode()
                self.send_response(code)
                self.send_header("Content-Type", "application/json")
                self.send_header("Content-Length", str(len(b)))
                self.end_headers()
                self.wfile.write(b)

            def do_GET(self):
                u = urlparse(self.path)
                p = {k: v[0] for k, v in parse_qs(u.query).items()}
                if u.path == "/-/ready":
                    return self._send({"ok": True})
                outer.queries.append(p.get("query", ""))
                if u.path == "/api/v1/query_range":
                    vals = outer.range_fn(p["query"], float(p["start"]), float(p["end"]), int(p["step"].rstrip("s")))
                    res = [{"metric": {}, "values": [[t, str(v)] for t, v in vals]}] if vals is not None else []
                    return self._send({"status": "success", "data": {"resultType": "matrix", "result": res}})
                if u.path == "/api/v1/query":
                    v = outer.instant_fn(p["query"])
                    res = [] if v is None else [{"metric": {}, "value": [1.0, str(v)]}]
                    return self._send({"status": "success", "data": {"resultType": "vector", "result": res}})
                self._send({}, 404)

        self.httpd = ThreadingHTTPServer(("127.0.0.1", 0), H)
        self.url = f"http://127.0.0.1:{self.httpd.server_address[1]}"
        self.thread = threading.Thread(target=self.httpd.serve_forever, daemon=True)

    def __enter__(self):
        self.thread.start()
        return self

    def __exit__(self, *a):
        self.httpd.shutdown()
        self.httpd.server_close()
