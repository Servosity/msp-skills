.PHONY: build test lint install clean

BIN_EXT := $(if $(filter windows,$(shell go env GOOS)),.exe,)

build:
	go build -trimpath -o bin/dropsuite-cli$(BIN_EXT) ./cmd/dropsuite-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/dropsuite-cli

clean:
	rm -rf bin/

build-mcp:
	go build -trimpath -o bin/dropsuite-mcp$(BIN_EXT) ./cmd/dropsuite-mcp

install-mcp:
	go install ./cmd/dropsuite-mcp

build-all: build build-mcp
