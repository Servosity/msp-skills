.PHONY: build test lint install clean

BIN_EXT := $(if $(filter windows,$(shell go env GOOS)),.exe,)

build:
	go build -trimpath -o bin/liongard-cli$(BIN_EXT) ./cmd/liongard-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/liongard-cli

clean:
	rm -rf bin/

build-mcp:
	go build -trimpath -o bin/liongard-mcp$(BIN_EXT) ./cmd/liongard-mcp

install-mcp:
	go install ./cmd/liongard-mcp

build-all: build build-mcp
