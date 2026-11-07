.PHONY: build test lint install clean

BIN_EXT := $(if $(filter windows,$(shell go env GOOS)),.exe,)

build:
	go build -trimpath -o bin/tactical-rmm-cli$(BIN_EXT) ./cmd/tactical-rmm-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/tactical-rmm-cli

clean:
	rm -rf bin/

build-mcp:
	go build -trimpath -o bin/tactical-rmm-mcp$(BIN_EXT) ./cmd/tactical-rmm-mcp

install-mcp:
	go install ./cmd/tactical-rmm-mcp

build-all: build build-mcp
