# Dev: dummy services (Phase 1)
Local (repo root): `pip install -r services/requirements.txt && bash services/run_local.sh`
Compose: `docker compose -f services/docker-compose.services.yml up --build`
Smoke: `curl -s -XPOST localhost:8000/orders -H 'content-type: application/json' -d '{"customer_id":"c1","item_id":"i1","quantity":1,"amount":10}'`
Faults: `scripts/faults/fault.sh latency payments 800` | `error payments 0.5` | `refuse` | `restore` (MODE=docker for Compose)
`services/common/models.py` mirrors contract DTOs; swap to `minidd_contracts.models` after v1.0.0.
