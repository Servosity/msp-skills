.PHONY: build test lint install clean

build:
	go build -o bin/ninjaone-cli ./cmd/ninjaone-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/ninjaone-cli

clean:
	rm -rf bin/

build-mcp:
	go build -o bin/ninjaone-mcp ./cmd/ninjaone-mcp

install-mcp:
	go install ./cmd/ninjaone-mcp

build-all: build build-mcp
