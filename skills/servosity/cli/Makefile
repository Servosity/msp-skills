.PHONY: build test lint install clean

build:
	go build -o bin/servosity-cli ./cmd/servosity-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/servosity-cli

clean:
	rm -rf bin/

build-mcp:
	go build -o bin/servosity-mcp ./cmd/servosity-mcp

install-mcp:
	go install ./cmd/servosity-mcp

build-all: build build-mcp
