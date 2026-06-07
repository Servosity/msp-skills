.PHONY: build test lint install clean

build:
	go build -o bin/levelio-cli ./cmd/levelio-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/levelio-cli

clean:
	rm -rf bin/

build-mcp:
	go build -o bin/levelio-mcp ./cmd/levelio-mcp

install-mcp:
	go install ./cmd/levelio-mcp

build-all: build build-mcp
