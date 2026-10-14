.PHONY: build test lint install clean

BIN_EXT := $(if $(filter windows,$(shell go env GOOS)),.exe,)

build:
	go build -trimpath -o bin/sophos-central-cli$(BIN_EXT) ./cmd/sophos-central-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/sophos-central-cli

clean:
	rm -rf bin/

build-mcp:
	go build -trimpath -o bin/sophos-central-mcp$(BIN_EXT) ./cmd/sophos-central-mcp

install-mcp:
	go install ./cmd/sophos-central-mcp

build-all: build build-mcp
