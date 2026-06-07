.PHONY: build test lint install clean

build:
	go build -o bin/action1-cli ./cmd/action1-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/action1-cli

clean:
	rm -rf bin/

build-mcp:
	go build -o bin/action1-mcp ./cmd/action1-mcp

install-mcp:
	go install ./cmd/action1-mcp

build-all: build build-mcp
