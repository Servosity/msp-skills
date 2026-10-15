.PHONY: build test lint install clean

BIN_EXT := $(if $(filter windows,$(shell go env GOOS)),.exe,)

build:
	go build -trimpath -o bin/connectsecure-cli$(BIN_EXT) ./cmd/connectsecure-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/connectsecure-cli

clean:
	rm -rf bin/

build-mcp:
	go build -trimpath -o bin/connectsecure-mcp$(BIN_EXT) ./cmd/connectsecure-mcp

install-mcp:
	go install ./cmd/connectsecure-mcp

build-all: build build-mcp
