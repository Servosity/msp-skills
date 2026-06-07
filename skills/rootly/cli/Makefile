.PHONY: build test lint install clean

build:
	go build -o bin/rootly-cli ./cmd/rootly-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/rootly-cli

clean:
	rm -rf bin/

build-mcp:
	go build -o bin/rootly-mcp ./cmd/rootly-mcp

install-mcp:
	go install ./cmd/rootly-mcp

build-all: build build-mcp
