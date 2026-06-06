.PHONY: build test lint install clean

build:
	go build -o bin/pax8-cli ./cmd/pax8-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/pax8-cli

clean:
	rm -rf bin/

build-mcp:
	go build -o bin/pax8-mcp ./cmd/pax8-mcp

install-mcp:
	go install ./cmd/pax8-mcp

build-all: build build-mcp
