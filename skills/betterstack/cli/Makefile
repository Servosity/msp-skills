.PHONY: build test lint install clean

BIN_EXT := $(if $(filter windows,$(shell go env GOOS)),.exe,)

build:
	go build -trimpath -o bin/betterstack-cli$(BIN_EXT) ./cmd/betterstack-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/betterstack-cli

clean:
	rm -rf bin/

build-mcp:
	go build -trimpath -o bin/betterstack-mcp$(BIN_EXT) ./cmd/betterstack-mcp

install-mcp:
	go install ./cmd/betterstack-mcp

build-all: build build-mcp
