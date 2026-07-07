.PHONY: test test-race cover cover-html lint fmt db-up db-down

COVERPKGS = $(shell go list ./... | grep -v /internal/postgres/db)

test:
	go test ./...

test-race:
	go test -race ./...

lint:
	golangci-lint run

fmt:
	golangci-lint fmt

db-up:
	docker compose up -d --wait

db-down:
	docker compose down

cover:
	go test -cover $(COVERPKGS)

cover-html:
	go test -coverprofile=cover.out $(COVERPKGS)
	go tool cover -html=cover.out
