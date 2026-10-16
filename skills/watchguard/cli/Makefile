.PHONY: build test lint install clean

BIN_EXT := $(if $(filter windows,$(shell go env GOOS)),.exe,)

build:
	go build -trimpath -o bin/watchguard-cli$(BIN_EXT) ./cmd/watchguard-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/watchguard-cli

clean:
	rm -rf bin/

build-mcp:
	go build -trimpath -o bin/watchguard-mcp$(BIN_EXT) ./cmd/watchguard-mcp

install-mcp:
	go install ./cmd/watchguard-mcp

build-all: build build-mcp
