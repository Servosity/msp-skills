.PHONY: build test lint install clean

build:
	go build -o bin/sherweb-cli ./cmd/sherweb-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/sherweb-cli

clean:
	rm -rf bin/

build-mcp:
	go build -o bin/sherweb-mcp ./cmd/sherweb-mcp

install-mcp:
	go install ./cmd/sherweb-mcp

build-all: build build-mcp
