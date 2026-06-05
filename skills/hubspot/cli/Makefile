.PHONY: build test lint install clean

build:
	go build -o bin/hubspot-cli ./cmd/hubspot-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/hubspot-cli

clean:
	rm -rf bin/

build-mcp:
	go build -o bin/hubspot-mcp ./cmd/hubspot-mcp

install-mcp:
	go install ./cmd/hubspot-mcp

build-all: build build-mcp
