.PHONY: build test lint install clean

build:
	go build -o bin/huntress-cli ./cmd/huntress-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/huntress-cli

clean:
	rm -rf bin/

build-mcp:
	go build -o bin/huntress-mcp ./cmd/huntress-mcp

install-mcp:
	go install ./cmd/huntress-mcp

build-all: build build-mcp
