.PHONY: build test lint install clean

build:
	go build -o bin/riverside-fm-cli ./cmd/riverside-fm-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/riverside-fm-cli

clean:
	rm -rf bin/

build-mcp:
	go build -o bin/riverside-fm-mcp ./cmd/riverside-fm-mcp

install-mcp:
	go install ./cmd/riverside-fm-mcp

build-all: build build-mcp
