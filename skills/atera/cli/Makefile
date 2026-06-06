.PHONY: build test lint install clean

build:
	go build -o bin/atera-cli ./cmd/atera-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/atera-cli

clean:
	rm -rf bin/

build-mcp:
	go build -o bin/atera-mcp ./cmd/atera-mcp

install-mcp:
	go install ./cmd/atera-mcp

build-all: build build-mcp
