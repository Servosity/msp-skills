.PHONY: build test lint install clean

BIN_EXT := $(if $(filter windows,$(shell go env GOOS)),.exe,)

build:
	go build -o bin/resourceguru-cli$(BIN_EXT) ./cmd/resourceguru-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/resourceguru-cli

clean:
	rm -rf bin/

build-mcp:
	go build -o bin/resourceguru-mcp$(BIN_EXT) ./cmd/resourceguru-mcp

install-mcp:
	go install ./cmd/resourceguru-mcp

build-all: build build-mcp
