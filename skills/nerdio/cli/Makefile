.PHONY: build test lint install clean

build:
	go build -o bin/nerdio-cli ./cmd/nerdio-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/nerdio-cli

clean:
	rm -rf bin/

build-mcp:
	go build -o bin/nerdio-mcp ./cmd/nerdio-mcp

install-mcp:
	go install ./cmd/nerdio-mcp

build-all: build build-mcp
