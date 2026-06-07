.PHONY: build test lint install clean

build:
	go build -o bin/runzero-cli ./cmd/runzero-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/runzero-cli

clean:
	rm -rf bin/

build-mcp:
	go build -o bin/runzero-mcp ./cmd/runzero-mcp

install-mcp:
	go install ./cmd/runzero-mcp

build-all: build build-mcp
