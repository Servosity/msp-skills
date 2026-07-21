.PHONY: build test lint install clean

BIN_EXT := $(if $(filter windows,$(shell go env GOOS)),.exe,)

build:
	go build -o bin/granola-cli$(BIN_EXT) ./cmd/granola-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/granola-cli

clean:
	rm -rf bin/

build-mcp:
	go build -o bin/granola-mcp$(BIN_EXT) ./cmd/granola-mcp

install-mcp:
	go install ./cmd/granola-mcp

build-all: build build-mcp
