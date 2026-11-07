.PHONY: build test lint install clean

BIN_EXT := $(if $(filter windows,$(shell go env GOOS)),.exe,)

build:
	go build -trimpath -o bin/pax8-cli$(BIN_EXT) ./cmd/pax8-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/pax8-cli

clean:
	rm -rf bin/

build-mcp:
	go build -trimpath -o bin/pax8-mcp$(BIN_EXT) ./cmd/pax8-mcp

install-mcp:
	go install ./cmd/pax8-mcp

build-all: build build-mcp
