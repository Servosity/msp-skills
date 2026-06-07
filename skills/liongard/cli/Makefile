.PHONY: build test lint install clean

build:
	go build -o bin/liongard-cli ./cmd/liongard-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/liongard-cli

clean:
	rm -rf bin/

build-mcp:
	go build -o bin/liongard-mcp ./cmd/liongard-mcp

install-mcp:
	go install ./cmd/liongard-mcp

build-all: build build-mcp
