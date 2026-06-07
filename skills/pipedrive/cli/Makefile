.PHONY: build test lint install clean

build:
	go build -o bin/pipedrive-cli ./cmd/pipedrive-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/pipedrive-cli

clean:
	rm -rf bin/

build-mcp:
	go build -o bin/pipedrive-mcp ./cmd/pipedrive-mcp

install-mcp:
	go install ./cmd/pipedrive-mcp

build-all: build build-mcp
