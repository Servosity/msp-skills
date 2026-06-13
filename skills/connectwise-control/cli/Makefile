.PHONY: build test lint install clean

build:
	go build -o bin/connectwise-control-cli ./cmd/connectwise-control-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/connectwise-control-cli

clean:
	rm -rf bin/

build-mcp:
	go build -o bin/connectwise-control-mcp ./cmd/connectwise-control-mcp

install-mcp:
	go install ./cmd/connectwise-control-mcp

build-all: build build-mcp
