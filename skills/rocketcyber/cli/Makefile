.PHONY: build test lint install clean

build:
	go build -o bin/rocketcyber-cli ./cmd/rocketcyber-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/rocketcyber-cli

clean:
	rm -rf bin/

build-mcp:
	go build -o bin/rocketcyber-mcp ./cmd/rocketcyber-mcp

install-mcp:
	go install ./cmd/rocketcyber-mcp

build-all: build build-mcp
