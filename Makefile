.PHONY: peers test test-race cover cover-html lint vuln fmt generate outdated db-up db-down db-reset \
	seed dev dev-watch demo n8n n8n-down n8n-node n8n-node-local runbook \
	e2e e2e-build e2e-serve e2e-db-reset e2e-seed e2e-reset

COVERPKGS = $(shell go list ./... | grep -v -e /internal/postgres/db -e /internal/testdb -e /internal/graphroot -e '/alphone/graph$$' -e '/alphone/graph/model$$')

test:
	go test ./...

test-race:
	go test -race ./...

peers:
	pnpm peers check

lint:
	golangci-lint run
	go run ./cmd/doclint

vuln:
	GOWORK=off go tool govulncheck ./...

fmt:
	golangci-lint fmt

generate:
	go run ./cmd/pluginwire
	GOWORK=off go tool sqlc generate
	GOWORK=off go tool gqlgen generate
	go run ./cmd/schemagen
	pnpm exec graphql-codegen --config codegen.ts

pot:
	pnpm --filter @alphone/frontend exec node scripts/write-pot.ts

catalogs:
	pnpm --filter @alphone/frontend exec node scripts/write-catalogs.ts

translations:
	cd frontend && node --env-file-if-exists=$(CURDIR)/.env scripts/sync-translations.ts

translations-retire:
	cd frontend && node --env-file-if-exists=$(CURDIR)/.env scripts/retire-translations.ts

outdated:
	@echo "=== direct Go modules with updates ==="
	@go list -m -u -f '{{if and (not .Indirect) .Update}}  {{.Path}}: {{.Version}} -> {{.Update.Version}}{{end}}' all 2>/dev/null | grep . || echo "  (all current)"
	@echo "=== npm packages with updates (workspace) ==="
	@pnpm -r outdated 2>/dev/null || true
	@echo "=== go tool dependencies (versioned in go.mod) ==="
	@go list -m -u -f '{{if .Update}}  {{.Path}}: {{.Version}} -> {{.Update.Version}}{{else}}  {{.Path}}: {{.Version}} (current){{end}}' \
		$$(sed -n '/^tool (/,/^)/p' go.mod | grep -v '^tool\|^)' | tr -d '\t ' \
			| xargs -r -n1 go list -f '{{.Module.Path}}' 2>/dev/null | sort -u) 2>/dev/null || true
	@echo "=== pinned tools to review by hand ==="
	@echo "  go directive / installed:  $$(sed -n 's/^go //p' go.mod) / $$(go env GOVERSION)"
	@echo "  also: golangci-lint-action + setup-go + pnpm packageManager,"
	@echo "        the postgres docker image, and @wordpress/* (curated vs CHANGELOG)"

db-up:
	docker compose up -d --wait

db-down:
	docker compose down

db-reset: db-up
	docker compose exec -T postgres psql -U postgres -d postgres -v ON_ERROR_STOP=1 \
		-c "DROP SCHEMA IF EXISTS core, auth, plugin_importer, plugin_whatsapp CASCADE" \
		-c "DROP TABLE IF EXISTS public.goose_db_version"
	$(MAKE) seed

n8n:
	docker compose --profile n8n up -d --wait n8n
	@echo "n8n at http://localhost:5678"

n8n-down:
	docker compose --profile n8n down n8n

N8N_NODE ?= n8n-nodes-alphone

n8n-node:
	docker compose exec -T n8n sh -c 'cd /home/node/.n8n/nodes && npm uninstall $(N8N_NODE) || true'
	docker compose exec -T n8n sh -c 'cd /home/node/.n8n/nodes && npm install $(N8N_NODE)@latest'
	docker compose --profile n8n restart n8n
	@docker compose exec -T n8n sh -c 'cd /home/node/.n8n/nodes && npm ls $(N8N_NODE)' || true

N8N_NODE_DIR ?= ../n8n-nodes-alphone

n8n-node-local:
	cd $(N8N_NODE_DIR) && npx n8n-node build && npm pack
	docker compose cp $(N8N_NODE_DIR)/$(N8N_NODE)-*.tgz n8n:/tmp/node.tgz
	docker compose exec -T n8n sh -c 'cd /home/node/.n8n/nodes && npm install /tmp/node.tgz'
	docker compose --profile n8n restart n8n
	@docker compose exec -T n8n sh -c \
		'grep -c "api/graphql" /home/node/.n8n/nodes/node_modules/$(N8N_NODE)/dist/nodes/AlphOne/graph.js' \
		&& echo "the graph build is installed" || echo "the graph build is NOT installed"

runbook:
	@test -n "$(ALPHONE_TOKEN)" || \
		(echo "set ALPHONE_TOKEN, mint one with: ./alphone token create -email you@example.com -name runbook" && false)
	node test/runbook/run.ts

seed: db-up
	go run ./cmd/alphone seed

dev: db-up
	go run ./cmd/alphone

dev-watch: db-up
	GOWORK=off go tool air

demo: db-up e2e-build
	ALPHONE_WEB_DIR=frontend/dist ./alphone

COVERDATA = .covdata

cover:
	rm -rf $(COVERDATA)
	mkdir -p $(COVERDATA)/bin $(COVERDATA)/counters
	go build -cover -coverpkg=./cmd/... -o $(COVERDATA)/bin ./cmd/alphone ./cmd/doclint ./cmd/pluginwire ./cmd/schemagen
	ALPHONE_COVER_BINDIR=$(CURDIR)/$(COVERDATA)/bin \
	ALPHONE_COVER_GOCOVERDIR=$(CURDIR)/$(COVERDATA)/counters \
	go test -cover $(COVERPKGS) -args -test.gocoverdir=$(CURDIR)/$(COVERDATA)/counters
	@echo "=== merged unit + binary coverage ==="
	go tool covdata percent -i=$(COVERDATA)/counters
	@go tool covdata textfmt -i=$(COVERDATA)/counters -o $(COVERDATA)/cover.out
	@go tool cover -func=$(COVERDATA)/cover.out | tail -1

cover-html: cover
	go tool cover -html=$(COVERDATA)/cover.out

E2E_DB ?= alphone_e2e
E2E_DATABASE_URL ?= postgres://postgres:alphone@localhost:5433/$(E2E_DB)?sslmode=disable
E2E_EMAIL ?= e2e@example.com
E2E_NAME ?= Grace Hopper
E2E_PASSWORD ?= correct horse battery
E2E_ROLE ?= admin
E2E_WHATSAPP_APP_SECRET ?= e2e-app-secret
E2E_WHATSAPP_CREDENTIALS_KEY ?= 0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef
E2E_TENANT_MACHINE_GRACE ?= 0s
E2E_WHATSAPP_GRAPH_URL ?= http://127.0.0.1:4791
E2E_SMTP_PORT ?= 4792
E2E_PUBLIC_URL ?= http://localhost:8080

e2e-build:
	pnpm --filter @alphone/frontend build
	go build -o alphone ./cmd/alphone

e2e-serve: db-up e2e-build
	ALPHONE_WEB_DIR=frontend/dist ALPHONE_DATABASE_URL="$(E2E_DATABASE_URL)" \
		ALPHONE_WHATSAPP_APP_SECRET="$(E2E_WHATSAPP_APP_SECRET)" \
		ALPHONE_WHATSAPP_GRAPH_URL="$(E2E_WHATSAPP_GRAPH_URL)" \
		ALPHONE_WHATSAPP_ACCESS_TOKEN=e2e-not-a-real-token \
		ALPHONE_WHATSAPP_PHONE_NUMBER_ID=e2e-phone-number-id \
		ALPHONE_WHATSAPP_CREDENTIALS_KEY="$(E2E_WHATSAPP_CREDENTIALS_KEY)" \
		ALPHONE_TENANT_MACHINE_GRACE="$(E2E_TENANT_MACHINE_GRACE)" \
		ALPHONE_SMTP_HOST=127.0.0.1 \
		ALPHONE_SMTP_PORT="$(E2E_SMTP_PORT)" \
		ALPHONE_SMTP_FROM=crm@example.com \
		ALPHONE_SMTP_TLS=none \
		ALPHONE_PUBLIC_URL="$(E2E_PUBLIC_URL)" \
		./alphone

e2e-db-reset: db-up
	docker compose exec -T postgres psql -U postgres -v ON_ERROR_STOP=1 \
		-c "DROP DATABASE IF EXISTS $(E2E_DB) WITH (FORCE)" \
		-c "CREATE DATABASE $(E2E_DB)"

e2e-seed: db-up e2e-build
	printf '%s\n' "$(E2E_PASSWORD)" | \
		ALPHONE_DATABASE_URL="$(E2E_DATABASE_URL)" ./alphone createadmin \
		-email "$(E2E_EMAIL)" -name "$(E2E_NAME)" -role "$(E2E_ROLE)"

e2e-reset: e2e-db-reset e2e-seed

e2e: e2e-reset
	ALPHONE_E2E_DATABASE_URL="$(E2E_DATABASE_URL)" \
		pnpm --filter @alphone/e2e exec playwright test
