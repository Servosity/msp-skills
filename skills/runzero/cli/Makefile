.PHONY: build test lint install clean

BIN_EXT := $(if $(filter windows,$(shell go env GOOS)),.exe,)

build:
	go build -trimpath -o bin/runzero-cli$(BIN_EXT) ./cmd/runzero-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/runzero-cli

clean:
	rm -rf bin/

build-mcp:
	go build -trimpath -o bin/runzero-mcp$(BIN_EXT) ./cmd/runzero-mcp

install-mcp:
	go install ./cmd/runzero-mcp

build-all: build build-mcp
