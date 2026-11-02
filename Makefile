.PHONY: generate test build verify test-integration test-capacity
SQLC ?= sqlc

generate:
	$(SQLC) generate

test:
	go test -race ./...

build:
	npm run build
	mkdir -p bin
	CGO_ENABLED=0 go build -trimpath -o bin/kb ./cmd/kb

verify: generate build
	go vet ./...
	go test -race ./...
	npm run test:e2e

test-integration:
	python3 tests/integration/test_proxy.py

test-capacity:
	CGO_ENABLED=0 go build -o bin/kb-load ./tests/load
	python3 tests/load/capacity.py --seconds 600
