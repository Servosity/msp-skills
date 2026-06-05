.PHONY: build test lint install clean

build:
	go build -o bin/connectwise-manage-cli ./cmd/connectwise-manage-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/connectwise-manage-cli

clean:
	rm -rf bin/

build-mcp:
	go build -o bin/connectwise-manage-mcp ./cmd/connectwise-manage-mcp

install-mcp:
	go install ./cmd/connectwise-manage-mcp

build-all: build build-mcp
