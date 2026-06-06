.PHONY: build test lint install clean

build:
	go build -o bin/itglue-cli ./cmd/itglue-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/itglue-cli

clean:
	rm -rf bin/

build-mcp:
	go build -o bin/itglue-mcp ./cmd/itglue-mcp

install-mcp:
	go install ./cmd/itglue-mcp

build-all: build build-mcp
