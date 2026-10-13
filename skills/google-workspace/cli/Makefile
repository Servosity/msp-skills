.PHONY: build test lint install clean

BIN_EXT := $(if $(filter windows,$(shell go env GOOS)),.exe,)

build:
	go build -trimpath -o bin/google-workspace-cli$(BIN_EXT) ./cmd/google-workspace-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/google-workspace-cli

clean:
	rm -rf bin/

build-mcp:
	go build -trimpath -o bin/google-workspace-mcp$(BIN_EXT) ./cmd/google-workspace-mcp

install-mcp:
	go install ./cmd/google-workspace-mcp

build-all: build build-mcp
