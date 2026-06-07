.PHONY: build test lint install clean

build:
	go build -o bin/tactical-rmm-cli ./cmd/tactical-rmm-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/tactical-rmm-cli

clean:
	rm -rf bin/

build-mcp:
	go build -o bin/tactical-rmm-mcp ./cmd/tactical-rmm-mcp

install-mcp:
	go install ./cmd/tactical-rmm-mcp

build-all: build build-mcp
