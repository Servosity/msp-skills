.PHONY: build test lint install clean

build:
	go build -o bin/hudu-cli ./cmd/hudu-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/hudu-cli

clean:
	rm -rf bin/

build-mcp:
	go build -o bin/hudu-mcp ./cmd/hudu-mcp

install-mcp:
	go install ./cmd/hudu-mcp

build-all: build build-mcp
