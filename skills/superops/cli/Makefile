.PHONY: build test lint install clean

build:
	go build -o bin/superops-cli ./cmd/superops-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/superops-cli

clean:
	rm -rf bin/

build-mcp:
	go build -o bin/superops-mcp ./cmd/superops-mcp

install-mcp:
	go install ./cmd/superops-mcp

build-all: build build-mcp
