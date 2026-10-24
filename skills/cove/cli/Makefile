.PHONY: build test lint install clean

BIN_EXT := $(if $(filter windows,$(shell go env GOOS)),.exe,)

build:
	go build -trimpath -o bin/cove-cli$(BIN_EXT) ./cmd/cove-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/cove-cli

clean:
	rm -rf bin/

build-mcp:
	go build -trimpath -o bin/cove-mcp$(BIN_EXT) ./cmd/cove-mcp

install-mcp:
	go install ./cmd/cove-mcp

build-all: build build-mcp
