.PHONY: generate test build verify
SQLC ?= sqlc

generate:
	$(SQLC) generate

test:
	go test -race ./...

build:
	mkdir -p bin
	CGO_ENABLED=0 go build -trimpath -o bin/kb ./cmd/kb

verify: generate test build
	go vet ./...
