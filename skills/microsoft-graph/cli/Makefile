.PHONY: build test lint install clean

build:
	go build -o bin/microsoft-graph-cli ./cmd/microsoft-graph-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/microsoft-graph-cli

clean:
	rm -rf bin/

build-mcp:
	go build -o bin/microsoft-graph-mcp ./cmd/microsoft-graph-mcp

install-mcp:
	go install ./cmd/microsoft-graph-mcp

build-all: build build-mcp
