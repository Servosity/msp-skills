.PHONY: build test lint install clean

BIN_EXT := $(if $(filter windows,$(shell go env GOOS)),.exe,)

build:
	go build -trimpath -o bin/apple-business-cli$(BIN_EXT) ./cmd/apple-business-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/apple-business-cli

clean:
	rm -rf bin/

build-mcp:
	go build -trimpath -o bin/apple-business-mcp$(BIN_EXT) ./cmd/apple-business-mcp

install-mcp:
	go install ./cmd/apple-business-mcp

build-all: build build-mcp
