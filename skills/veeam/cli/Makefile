.PHONY: build test lint install clean

build:
	go build -o bin/veeam-cli ./cmd/veeam-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/veeam-cli

clean:
	rm -rf bin/

build-mcp:
	go build -o bin/veeam-mcp ./cmd/veeam-mcp

install-mcp:
	go install ./cmd/veeam-mcp

build-all: build build-mcp
