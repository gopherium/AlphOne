.PHONY: test cover cover-html

test:
	go test ./...

cover:
	go test -cover ./...

cover-html:
	go test -coverprofile=cover.out ./...
	go tool cover -html=cover.out
