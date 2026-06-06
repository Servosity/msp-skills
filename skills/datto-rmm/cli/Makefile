.PHONY: build test lint install clean

build:
	go build -o bin/datto-rmm-cli ./cmd/datto-rmm-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/datto-rmm-cli

clean:
	rm -rf bin/

build-mcp:
	go build -o bin/datto-rmm-mcp ./cmd/datto-rmm-mcp

install-mcp:
	go install ./cmd/datto-rmm-mcp

build-all: build build-mcp
