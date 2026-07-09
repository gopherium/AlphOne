.PHONY: test test-race cover cover-html lint fmt generate outdated db-up db-down

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

cover:
	go test -cover $(COVERPKGS)

cover-html:
	go test -coverprofile=cover.out $(COVERPKGS)
	go tool cover -html=cover.out
