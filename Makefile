.PHONY: test cover cover-html

COVERPKGS = $(shell go list ./... | grep -v /internal/postgres/db)

test:
	go test ./...

cover:
	go test -cover $(COVERPKGS)

cover-html:
	go test -coverprofile=cover.out $(COVERPKGS)
	go tool cover -html=cover.out
