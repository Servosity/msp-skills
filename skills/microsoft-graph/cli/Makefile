.PHONY: build test lint install clean

BIN_EXT := $(if $(filter windows,$(shell go env GOOS)),.exe,)

build:
	go build -trimpath -o bin/microsoft-graph-cli$(BIN_EXT) ./cmd/microsoft-graph-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/microsoft-graph-cli

clean:
	rm -rf bin/

build-mcp:
	go build -trimpath -o bin/microsoft-graph-mcp$(BIN_EXT) ./cmd/microsoft-graph-mcp

install-mcp:
	go install ./cmd/microsoft-graph-mcp

build-all: build build-mcp
