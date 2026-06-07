.PHONY: build test lint install clean

build:
	go build -o bin/xero-cli ./cmd/xero-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/xero-cli

clean:
	rm -rf bin/

build-mcp:
	go build -o bin/xero-mcp ./cmd/xero-mcp

install-mcp:
	go install ./cmd/xero-mcp

build-all: build build-mcp
