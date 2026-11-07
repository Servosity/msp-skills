.PHONY: build test lint install clean

BIN_EXT := $(if $(filter windows,$(shell go env GOOS)),.exe,)

build:
	go build -trimpath -o bin/xero-cli$(BIN_EXT) ./cmd/xero-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/xero-cli

clean:
	rm -rf bin/

build-mcp:
	go build -trimpath -o bin/xero-mcp$(BIN_EXT) ./cmd/xero-mcp

install-mcp:
	go install ./cmd/xero-mcp

build-all: build build-mcp
