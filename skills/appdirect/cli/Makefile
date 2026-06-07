.PHONY: build test lint install clean

build:
	go build -o bin/appdirect-cli ./cmd/appdirect-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/appdirect-cli

clean:
	rm -rf bin/

build-mcp:
	go build -o bin/appdirect-mcp ./cmd/appdirect-mcp

install-mcp:
	go install ./cmd/appdirect-mcp

build-all: build build-mcp
