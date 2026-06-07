.PHONY: build test lint install clean

build:
	go build -o bin/gradient-cli ./cmd/gradient-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/gradient-cli

clean:
	rm -rf bin/

build-mcp:
	go build -o bin/gradient-mcp ./cmd/gradient-mcp

install-mcp:
	go install ./cmd/gradient-mcp

build-all: build build-mcp
