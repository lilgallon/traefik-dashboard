COMPOSE := docker compose -f dev/compose.yml

.PHONY: dev dev-logs dev-down screenshots test

## Local environment: http://localhost:18080 (dashboard), http://localhost:18081 (alerts)
dev:
	$(COMPOSE) up -d --build --wait traefik-dashboard telegram-mock
	@echo "Dashboard: http://localhost:18080  ·  alerts: http://localhost:18081"

dev-logs:
	$(COMPOSE) logs -f

## Stop the local environment and delete its volumes (history is regenerated)
dev-down:
	$(COMPOSE) down -v --remove-orphans

## README screenshots, written to docs/screenshots/ (needs `make dev`)
screenshots:
	$(COMPOSE) run --rm screenshots

test:
	go test ./...
	cd web && npm ci && npm run check && npm test
