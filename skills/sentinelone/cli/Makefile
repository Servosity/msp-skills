.PHONY: build test lint install clean

build:
	go build -o bin/sentinelone-cli ./cmd/sentinelone-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/sentinelone-cli

clean:
	rm -rf bin/

build-mcp:
	go build -o bin/sentinelone-mcp ./cmd/sentinelone-mcp

install-mcp:
	go install ./cmd/sentinelone-mcp

build-all: build build-mcp
