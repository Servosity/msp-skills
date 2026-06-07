.PHONY: build test lint install clean

build:
	go build -o bin/proofpoint-cli ./cmd/proofpoint-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/proofpoint-cli

clean:
	rm -rf bin/

build-mcp:
	go build -o bin/proofpoint-mcp ./cmd/proofpoint-mcp

install-mcp:
	go install ./cmd/proofpoint-mcp

build-all: build build-mcp
