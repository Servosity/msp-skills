.PHONY: build test lint install clean

build:
	go build -o bin/cipp-cli ./cmd/cipp-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/cipp-cli

clean:
	rm -rf bin/

build-mcp:
	go build -o bin/cipp-mcp ./cmd/cipp-mcp

install-mcp:
	go install ./cmd/cipp-mcp

build-all: build build-mcp
