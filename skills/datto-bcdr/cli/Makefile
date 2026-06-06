.PHONY: build test lint install clean

build:
	go build -o bin/datto-bcdr-cli ./cmd/datto-bcdr-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/datto-bcdr-cli

clean:
	rm -rf bin/

build-mcp:
	go build -o bin/datto-bcdr-mcp ./cmd/datto-bcdr-mcp

install-mcp:
	go install ./cmd/datto-bcdr-mcp

build-all: build build-mcp
