.PHONY: build test lint install clean

BIN_EXT := $(if $(filter windows,$(shell go env GOOS)),.exe,)

build:
	go build -o bin/servosity-cli$(BIN_EXT) ./cmd/servosity-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/servosity-cli

clean:
	rm -rf bin/

build-mcp:
	go build -o bin/servosity-mcp$(BIN_EXT) ./cmd/servosity-mcp

install-mcp:
	go install ./cmd/servosity-mcp

build-all: build build-mcp
