# Questions the supervisor will ask — everyone

Answer from your own doc (`docs/members/<your-name>.md`). These are the shared ones;
anything specific to your component is in your file.

## "Walk me through the architecture."

One paragraph, then stop talking. Four FastAPI services in a chain, four data planes
reading them, one aggregation layer in front.

```
loadgen ──▶ api-gateway ──▶ orders ──▶ payments ──▶ inventory
                │             │           │            │
                └─────────────┴───────────┴────────────┘
                     every hop is a real HTTP request

  metrics-api  :9001 ◀── Prometheus scrapes the four services every 5s
  logs-api     :9002 ◀── log-shipper tails the shared JSONL volume into Elasticsearch
  topology-api :9003 ◀── eBPF agent posts kernel TCP events
        │
        ▼
  platform-api :9000  ── the single browser-facing entry point
        │
        ▼
  dashboard :3000  ── one static HTML file, no build step
```

Nobody's service talks to the browser. The dashboard only knows `platform-api`.

## "Is this real data or fixtures?"

Real. Every number on screen comes from a live request through the chain.

- Metrics: Prometheus scraping live `/metrics` endpoints.
- Logs: JSONL written by the services as they handle requests, shipped to Elasticsearch.
- Network: either real eBPF reading the kernel, or a clearly-labelled simulator.

The only fixture data in the repo is `darsan/contracts/fixtures/fixtures.json`, used by
`MOCK=true` so the UI could be built before the backends existed. It is not running now.
The dashboard has a `?mock=1` mode that serves fixture-shaped data in the browser with no
backend at all — useful for UI work, never for the demo.

If you want to prove it live: `make -C demo walkthrough` sends a real request and prints
the 201.

## "What happens if a service goes down?"

Point at the Overview tab and run `make -C demo fault-refuse`. The chain degrades visibly
at three levels at once: ERROR logs appear from orders carrying
`ConnectionRefusedError: [Errno 111]`, and the `orders → payments` edge on the Network tab
turns red as the collector counts the failed connections.

That is the point of the demo. One injected fault, independent signals agreeing, with no
correlation engine to stitch them together — that is Phase 2.

If you want the error-rate number to move as well, use `make -C demo fault-error` instead:
that one returns real 5xx responses, so the rate genuinely climbs. During the refusal step
the error rate can *fall*, because Prometheus is on a 30-second window still holding the
5xx from an earlier fault. Know that in advance so you are not caught out by it.

## "What is not done?"

Say this plainly, it is in the plan as Phase 2/3:

- Anomaly detection (Dev) — rolling baseline, z-score, consecutive-breach cooldown
- Correlation engine (Diya) — anomaly + nearby logs + network events → one scored Incident
- Topology graph rendering as a real force layout, edge persistence (Brajesh)
- Kubernetes manifests, config management (Darsan)

The dashboard has an **"Incidents (Phase 2, planned)"** button that is visibly disabled.
Point at it. Better to show the boundary deliberately than be found out by a question.

## "How do you know the components actually talk to each other?"

Two independent answers, both runnable:

```bash
make -C demo smoke        # validates every Platform API response against the pydantic contract
make -C demo preflight    # 2-second go/no-go, ends in READY
```

`smoke` is the referee: it does not check status codes, it parses each response into the
contract's pydantic models, so contract drift fails the test rather than showing up as a
blank panel during the demo.

The contract is frozen in `02_CONTRACTS.md` + `darsan/contracts/` with a pinned
`CONTRACT_VERSION`, and it was committed **first**, before any component. In the history
it is the first Phase 1 commit, ahead of all five modules:

```
6d43992 Add root .gitignore ...
6dbed08 Add the frozen Phase 1 contracts, registry and fixtures   <-- first
d246787 Add the four microservices and the load generator
706fd13 Add the metrics API backed by Prometheus
a13ef44 Add the log shipper, search API and Elasticsearch index template
32074a4 Add the Platform API that fronts the four data APIs
4c42b24 Add the eBPF TCP collector and agent
```

The commit order is the argument: nothing was written until the surface was agreed.

## "Why one machine?"

eBPF needs a real Linux kernel, so this cannot be a set of hosted containers. One
Linux VM, Docker Compose, 14 services. See `01_PLAN_AND_DEMO.md` for the sizing.

## "What is the weakest part?"

Worth volunteering. The honest answers:

- The eBPF collector classifies connect-failed as refused-vs-timeout **by how fast the
  failure happened**, because the tracepoint does not expose `sk_err`. It is a heuristic,
  and the caveat sits in the code next to it.
- The dashboard's trace-by-request-id filters client-side over the newest 1000 log lines,
  so it only reaches back about 45 seconds at 5 rps.
- The topology buffer is in-memory, so restarting the Topology API clears the graph.
- The refusal fault needs a small supervisor (see Dev's doc). The obvious approach,
  `docker stop payments`, does not produce a real refusal at all.
