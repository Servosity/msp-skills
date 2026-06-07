.PHONY: build test lint install clean

build:
	go build -o bin/pandadoc-cli ./cmd/pandadoc-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/pandadoc-cli

clean:
	rm -rf bin/

build-mcp:
	go build -o bin/pandadoc-mcp ./cmd/pandadoc-mcp

install-mcp:
	go install ./cmd/pandadoc-mcp

build-all: build build-mcp
