.PHONY: test test-race cover cover-html lint fmt generate db-up db-down

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

db-up:
	docker compose up -d --wait

db-down:
	docker compose down

cover:
	go test -cover $(COVERPKGS)

cover-html:
	go test -coverprofile=cover.out $(COVERPKGS)
	go tool cover -html=cover.out
