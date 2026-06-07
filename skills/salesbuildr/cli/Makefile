.PHONY: build test lint install clean

build:
	go build -o bin/salesbuildr-cli ./cmd/salesbuildr-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/salesbuildr-cli

clean:
	rm -rf bin/

build-mcp:
	go build -o bin/salesbuildr-mcp ./cmd/salesbuildr-mcp

install-mcp:
	go install ./cmd/salesbuildr-mcp

build-all: build build-mcp
