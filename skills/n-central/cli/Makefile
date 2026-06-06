.PHONY: build test lint install clean

build:
	go build -o bin/n-central-cli ./cmd/n-central-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/n-central-cli

clean:
	rm -rf bin/

build-mcp:
	go build -o bin/n-central-mcp ./cmd/n-central-mcp

install-mcp:
	go install ./cmd/n-central-mcp

build-all: build build-mcp
