.PHONY: build test lint install clean

BIN_EXT := $(if $(filter windows,$(shell go env GOOS)),.exe,)

build:
	go build -trimpath -o bin/ninjaone-cli$(BIN_EXT) ./cmd/ninjaone-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/ninjaone-cli

clean:
	rm -rf bin/

build-mcp:
	go build -trimpath -o bin/ninjaone-mcp$(BIN_EXT) ./cmd/ninjaone-mcp

install-mcp:
	go install ./cmd/ninjaone-mcp

build-all: build build-mcp
