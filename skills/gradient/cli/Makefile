.PHONY: build test lint install clean

BIN_EXT := $(if $(filter windows,$(shell go env GOOS)),.exe,)

build:
	go build -trimpath -o bin/gradient-cli$(BIN_EXT) ./cmd/gradient-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/gradient-cli

clean:
	rm -rf bin/

build-mcp:
	go build -trimpath -o bin/gradient-mcp$(BIN_EXT) ./cmd/gradient-mcp

install-mcp:
	go install ./cmd/gradient-mcp

build-all: build build-mcp
