.PHONY: build test lint install clean

build:
	go build -o bin/betterstack-cli ./cmd/betterstack-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/betterstack-cli

clean:
	rm -rf bin/

build-mcp:
	go build -o bin/betterstack-mcp ./cmd/betterstack-mcp

install-mcp:
	go install ./cmd/betterstack-mcp

build-all: build build-mcp
