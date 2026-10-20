.PHONY: build test lint install clean

BIN_EXT := $(if $(filter windows,$(shell go env GOOS)),.exe,)

build:
	go build -trimpath -o bin/synology-cli$(BIN_EXT) ./cmd/synology-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/synology-cli

clean:
	rm -rf bin/

build-mcp:
	go build -trimpath -o bin/synology-mcp$(BIN_EXT) ./cmd/synology-mcp

install-mcp:
	go install ./cmd/synology-mcp

build-all: build build-mcp
