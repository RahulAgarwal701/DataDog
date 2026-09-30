C=docker compose -f deploy/docker-compose.yml
.PHONY: demo down smoke fault-refuse fault-restore mock contracts
demo:            ## start everything and wait for platform-api health
	$(C) up -d --build
	@echo "waiting for platform-api..."; for i in $$(seq 1 90); do curl -sf localhost:9000/health >/dev/null && echo READY && exit 0; sleep 2; done; echo "TIMEOUT"; exit 1
down:
	$(C) down -v
smoke:           ## validate every Platform API response against the contract models
	python scripts/smoke.py
fault-refuse:    ## connection refusal: stop payments
	$(C) stop payments
fault-restore:
	$(C) start payments
mock:            ## UI-only dev: open dashboard in mock mode
	python -m http.server 3000 -d dashboard   # then open http://localhost:3000/?mock=1
contracts:
	sh scripts/gen_contracts.sh
