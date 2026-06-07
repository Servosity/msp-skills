.PHONY: build test lint install clean

build:
	go build -o bin/cove-cli ./cmd/cove-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/cove-cli

clean:
	rm -rf bin/

build-mcp:
	go build -o bin/cove-mcp ./cmd/cove-mcp

install-mcp:
	go install ./cmd/cove-mcp

build-all: build build-mcp
