.PHONY: build test lint install clean

build:
	go build -o bin/maxio-cli ./cmd/maxio-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/maxio-cli

clean:
	rm -rf bin/

build-mcp:
	go build -o bin/maxio-mcp ./cmd/maxio-mcp

install-mcp:
	go install ./cmd/maxio-mcp

build-all: build build-mcp
