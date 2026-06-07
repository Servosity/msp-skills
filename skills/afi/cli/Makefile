.PHONY: build test lint install clean

build:
	go build -o bin/afi-cli ./cmd/afi-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/afi-cli

clean:
	rm -rf bin/

build-mcp:
	go build -o bin/afi-mcp ./cmd/afi-mcp

install-mcp:
	go install ./cmd/afi-mcp

build-all: build build-mcp
