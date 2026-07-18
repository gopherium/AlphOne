.PHONY: test test-race cover cover-html lint fmt generate outdated db-up db-down \
	e2e e2e-build e2e-serve e2e-db-reset e2e-seed e2e-reset

COVERPKGS = $(shell go list ./... | grep -v -e /internal/postgres/db -e /internal/testdb)

test:
	go test ./...

test-race:
	go test -race ./...

lint:
	golangci-lint run
	go run ./cmd/doclint

fmt:
	golangci-lint fmt

generate:
	go run ./cmd/pluginwire

outdated:
	@echo "=== direct Go modules with updates ==="
	@go list -m -u -f '{{if and (not .Indirect) .Update}}  {{.Path}}: {{.Version}} -> {{.Update.Version}}{{end}}' all 2>/dev/null | grep . || echo "  (all current)"
	@echo "=== npm packages with updates (workspace) ==="
	@pnpm -r outdated 2>/dev/null || true
	@echo "=== pinned tools to review by hand ==="
	@echo "  go directive / installed:  $$(sed -n 's/^go //p' go.mod) / $$(go env GOVERSION)"
	@echo "  also: golangci-lint-action + setup-go + pnpm packageManager,"
	@echo "        the postgres docker image, and @wordpress/* (curated vs CHANGELOG)"

db-up:
	docker compose up -d --wait

db-down:
	docker compose down

COVERDATA = .covdata

cover:
	rm -rf $(COVERDATA)
	mkdir -p $(COVERDATA)/bin $(COVERDATA)/counters
	go build -cover -coverpkg=./cmd/... -o $(COVERDATA)/bin ./cmd/alphone ./cmd/doclint ./cmd/pluginwire
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
E2E_WHATSAPP_APP_SECRET ?= e2e-app-secret
E2E_WHATSAPP_GRAPH_URL ?= http://127.0.0.1:4791

e2e-build:
	pnpm --filter @alphone/frontend build
	go build -o alphone ./cmd/alphone

e2e-serve: db-up e2e-build
	ALPHONE_WEB_DIR=frontend/dist ALPHONE_DATABASE_URL="$(E2E_DATABASE_URL)" \
		ALPHONE_WHATSAPP_APP_SECRET="$(E2E_WHATSAPP_APP_SECRET)" \
		ALPHONE_WHATSAPP_GRAPH_URL="$(E2E_WHATSAPP_GRAPH_URL)" \
		ALPHONE_WHATSAPP_ACCESS_TOKEN=e2e-not-a-real-token \
		ALPHONE_WHATSAPP_PHONE_NUMBER_ID=e2e-phone-number-id \
		./alphone

e2e-db-reset: db-up
	docker compose exec -T postgres psql -U postgres -v ON_ERROR_STOP=1 \
		-c "DROP DATABASE IF EXISTS $(E2E_DB) WITH (FORCE)" \
		-c "CREATE DATABASE $(E2E_DB)"

e2e-seed: db-up e2e-build
	printf '%s\n' "$(E2E_PASSWORD)" | \
		ALPHONE_DATABASE_URL="$(E2E_DATABASE_URL)" ./alphone createadmin \
		-email "$(E2E_EMAIL)" -name "$(E2E_NAME)"

e2e-reset: e2e-db-reset e2e-seed

e2e:
	pnpm --filter @alphone/e2e exec playwright test
