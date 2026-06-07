.PHONY: build test lint install clean

build:
	go build -o bin/blumira-cli ./cmd/blumira-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/blumira-cli

clean:
	rm -rf bin/

build-mcp:
	go build -o bin/blumira-mcp ./cmd/blumira-mcp

install-mcp:
	go install ./cmd/blumira-mcp

build-all: build build-mcp
