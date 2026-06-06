.PHONY: build test lint install clean

build:
	go build -o bin/acronis-cli ./cmd/acronis-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/acronis-cli

clean:
	rm -rf bin/

build-mcp:
	go build -o bin/acronis-mcp ./cmd/acronis-mcp

install-mcp:
	go install ./cmd/acronis-mcp

build-all: build build-mcp
