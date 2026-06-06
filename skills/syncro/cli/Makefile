.PHONY: build test lint install clean

build:
	go build -o bin/syncro-cli ./cmd/syncro-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/syncro-cli

clean:
	rm -rf bin/

build-mcp:
	go build -o bin/syncro-mcp ./cmd/syncro-mcp

install-mcp:
	go install ./cmd/syncro-mcp

build-all: build build-mcp
