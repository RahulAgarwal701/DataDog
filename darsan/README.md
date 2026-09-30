# Darsan's Phase 1 bundle
Copy these folders into the repo root (merge with the others): `contracts/ platform_api/ dashboard/ deploy/ scripts/ Makefile`.

- `pip install -e contracts/python` (everyone), `sh scripts/gen_contracts.sh` (JSON Schema)
- UI without backends: `make mock` then open http://localhost:3000/?mock=1
- Platform API alone: `pip install -r platform_api/requirements.txt -e contracts/python && python platform_api/main.py`
- Full stack: `make demo`, then `make smoke` (needs `pip install httpx pydantic`), dashboard at http://localhost:3000
- Faults: Demo Controls tab (latency/error), `make fault-refuse` / `make fault-restore` (connection refusal)
- eBPF agent runs on the host, not in Compose.

Compose assumes Dockerfile paths: services/{api_gateway,orders,payments,inventory,loadgen}/Dockerfile, metrics/metrics_api/Dockerfile, metrics/prometheus/prometheus.yml, topology/topology_api/Dockerfile. Adjust if teammates differ.
Not included (lean build): generated TS types, full fixture set, incident fixture, CI config.
