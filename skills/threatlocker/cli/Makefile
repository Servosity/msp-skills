.PHONY: build test lint install clean

build:
	go build -o bin/threatlocker-cli ./cmd/threatlocker-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/threatlocker-cli

clean:
	rm -rf bin/

build-mcp:
	go build -o bin/threatlocker-mcp ./cmd/threatlocker-mcp

install-mcp:
	go install ./cmd/threatlocker-mcp

build-all: build build-mcp
