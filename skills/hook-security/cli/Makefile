.PHONY: build test lint install clean

BIN_EXT := $(if $(filter windows,$(shell go env GOOS)),.exe,)

build:
	go build -trimpath -o bin/hook-security-cli$(BIN_EXT) ./cmd/hook-security-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/hook-security-cli

clean:
	rm -rf bin/

build-mcp:
	go build -trimpath -o bin/hook-security-mcp$(BIN_EXT) ./cmd/hook-security-mcp

install-mcp:
	go install ./cmd/hook-security-mcp

build-all: build build-mcp
