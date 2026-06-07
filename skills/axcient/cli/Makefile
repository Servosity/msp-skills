.PHONY: build test lint install clean

build:
	go build -o bin/axcient-cli ./cmd/axcient-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/axcient-cli

clean:
	rm -rf bin/

build-mcp:
	go build -o bin/axcient-mcp ./cmd/axcient-mcp

install-mcp:
	go install ./cmd/axcient-mcp

build-all: build build-mcp
