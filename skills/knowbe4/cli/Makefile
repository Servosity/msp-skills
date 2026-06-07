.PHONY: build test lint install clean

build:
	go build -o bin/knowbe4-cli ./cmd/knowbe4-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/knowbe4-cli

clean:
	rm -rf bin/

build-mcp:
	go build -o bin/knowbe4-mcp ./cmd/knowbe4-mcp

install-mcp:
	go install ./cmd/knowbe4-mcp

build-all: build build-mcp
