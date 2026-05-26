.PHONY: build test lint install clean

build:
	go build -o bin/halopsa-cli ./cmd/halopsa-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/halopsa-cli

clean:
	rm -rf bin/

build-mcp:
	go build -o bin/halopsa-mcp ./cmd/halopsa-mcp

install-mcp:
	go install ./cmd/halopsa-mcp

build-all: build build-mcp
