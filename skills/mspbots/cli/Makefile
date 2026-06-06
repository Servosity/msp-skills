.PHONY: build test lint install clean

build:
	go build -o bin/mspbots-cli ./cmd/mspbots-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/mspbots-cli

clean:
	rm -rf bin/

build-mcp:
	go build -o bin/mspbots-mcp ./cmd/mspbots-mcp

install-mcp:
	go install ./cmd/mspbots-mcp

build-all: build build-mcp
