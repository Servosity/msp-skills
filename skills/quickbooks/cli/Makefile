.PHONY: build test lint install clean

BIN_EXT := $(if $(filter windows,$(shell go env GOOS)),.exe,)

build:
	go build -trimpath -o bin/quickbooks-cli$(BIN_EXT) ./cmd/quickbooks-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/quickbooks-cli

clean:
	rm -rf bin/

build-mcp:
	go build -trimpath -o bin/quickbooks-mcp$(BIN_EXT) ./cmd/quickbooks-mcp

install-mcp:
	go install ./cmd/quickbooks-mcp

build-all: build build-mcp
