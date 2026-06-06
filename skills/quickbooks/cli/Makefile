.PHONY: build test lint install clean

build:
	go build -o bin/quickbooks-cli ./cmd/quickbooks-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/quickbooks-cli

clean:
	rm -rf bin/

build-mcp:
	go build -o bin/quickbooks-mcp ./cmd/quickbooks-mcp

install-mcp:
	go install ./cmd/quickbooks-mcp

build-all: build build-mcp
