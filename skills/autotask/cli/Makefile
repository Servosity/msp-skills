.PHONY: build test lint install clean

build:
	go build -o bin/autotask-cli ./cmd/autotask-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/autotask-cli

clean:
	rm -rf bin/

build-mcp:
	go build -o bin/autotask-mcp ./cmd/autotask-mcp

install-mcp:
	go install ./cmd/autotask-mcp

build-all: build build-mcp
