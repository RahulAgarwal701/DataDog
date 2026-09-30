# Darsan — contracts, Platform API, dashboard

**Folders you own:** `contracts/`, `platform_api/`, `dashboard/`, `scripts/`, CI
**Ports:** 9000 (Platform API), 3000 (dashboard)
**Phase 2/3:** topology graph view, incident list/detail, K8s manifests, config
management, error handling, report export

## What you built, in one paragraph

The contract, the thing everything else was written against, and the single
browser-facing entry point in front of the four data APIs. The dashboard is one static
HTML file with no build step. The Platform API aggregates health, proxies the four data
APIs, and owns the only write endpoint the UI touches — fault injection.

## The three deliverables

**`darsan/contracts/`** — the frozen surface

- `02_CONTRACTS.md` — the prose contract all five modules agreed on
- `registry.yaml` — authoritative service list: name, port, kind, `depends_on`
- `python/minidd_contracts/` — pydantic models
- `fixtures/fixtures.json` — shaped samples so the UI could be built before the backends
- `CONTRACT_VERSION` — pinned; changing it needs a bump in all five components

**`darsan/platform_api/`** — the aggregator

| Route | Purpose |
|---|---|
| `GET /health` | liveness |
| `GET /api/v1/services` | the registry, for the UI |
| `GET /api/v1/health/all` | every service's reachability in one call |
| `POST /api/v1/demo/fault` | the only write endpoint the dashboard uses |

Everything else is proxied: `/api/v1/metrics` → 9001, `/api/v1/logs` → 9002,
`/api/v1/topology` and `/api/v1/network-events` → 9003. Ports 9004/9005 (anomalies,
incidents) are already wired for Phase 2.

**`darsan/dashboard/`** — one `index.html`, nginx-served, five tabs: Overview, Metrics,
Logs, Network, Controls. `?api=` overrides the API base and `?mock=1` runs it entirely on
fixtures with no backend.

**`darsan/scripts/smoke.py`** — the referee. Parses every Platform API response into the
pydantic models. Not status codes; real validation.

## Why one aggregation layer

The browser only ever talks to `platform-api`. Two reasons: CORS lives in one place
instead of four, and the dashboard is decoupled from how many backends exist. When
Phase 2 adds the anomaly and correlation services, `index.html` does not change.

## How to demo it

```bash
make demo                  # starts everything, ends with a pass line per backend
make -C demo preflight     # 2-second go/no-go, must end in READY
```

Open **<http://localhost:3000>**. Walk the tabs. Then `make -C demo fault-latency` and
watch Overview and Metrics move.

## Questions you will actually get

**"How do you know the components fit together?"**

The contract, and the smoke test. `make -C demo smoke` validates every response against
the pydantic models, so if anyone drifts from the contract the test fails rather than the
panel going blank during the demo. The contract was committed *first* — it is the first
Phase 1 commit in the history, ahead of all five modules. The commit order is the
argument.

**"Why is the contract frozen?"**

Because five people were writing in parallel against it. `CONTRACT_VERSION` is pinned, and
a change needs sign-off from all five (contract doc, section 10). Without that, you get
six dialects of the same JSON.

**"The dashboard has no build step — is that a weakness?"**

For this phase, a feature. One file, no toolchain, no `npm install` before the demo. The
cost is that it is hand-written template strings. It is fine at five tabs and would not be
at thirty, which is when you would reach for a framework. The `?mock=1` mode is the
payoff: the whole UI is developable with no backend running at all.

**"Why does the dashboard call `:9000` and `:8010` directly instead of going through
nginx?"**

CORS is handled by the Platform API and loadgen, both of which allow `*`, and the
dashboard is served as a static file. Adding a reverse proxy in front of both would be the
production shape. For a demo on one machine it is a needless hop, and the preflight
verifies both origins answer, so it cannot silently break.

**"What happens if the Platform API is down?"**

The dashboard fails visibly — each tab shows the API error rather than an empty panel. That
is deliberate: an empty panel is ambiguous, an error is not. For the demo, if the Platform
API is down you can still show raw JSON (see `demo/DRIVE.md`).

**"Is the fault injection endpoint a security problem?"**

In this demo it is an open write endpoint on localhost that can degrade a service. It is
deliberate — it is the demo. It is in scope for the config-management and auth work in
Phase 3, and worth naming before you are asked.

## Things that will trip you up

- **Error rate `null` vs `0`.** A healthy service has no 5xx series, so `error_rate` is
  `null`. The dashboard renders that as `0.0 %`, because the panel is asking "how many
  errors" and the honest answer is none. The dash is reserved for metrics that genuinely
  have no data. Expect a question on this — it is a judgement call, not a bug.
- **`darsan/deploy/` is gone.** It referenced Dockerfiles at paths that never existed
  (`services/api_gateway/Dockerfile`) and was superseded by `demo/`. `darsan/Makefile` went
  with it. Do not go looking for them.
- **`make verify` had a real bug.** It requested `9000/health` with no hostname, so curl
  tried to resolve `9000` as a host and hung. `make demo` runs it, so a cold start hung
  before printing anything. Fixed to `localhost:PORT` with `--max-time`; same check went
  from a 60s timeout to 0.16s. Worth mentioning — it is the kind of thing that only shows
  up when you rehearse the cold path.

## Your own commands

```bash
make -C demo smoke            # the referee
make -C demo preflight        # go/no-go
make -C demo reset            # 5-second recovery
sh darsan/scripts/gen_contracts.sh   # regenerate models from the contract
```
