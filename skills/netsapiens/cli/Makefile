.PHONY: build test lint install clean

BIN_EXT := $(if $(filter windows,$(shell go env GOOS)),.exe,)

build:
	go build -trimpath -o bin/netsapiens-cli$(BIN_EXT) ./cmd/netsapiens-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/netsapiens-cli

clean:
	rm -rf bin/

build-mcp:
	go build -trimpath -o bin/netsapiens-mcp$(BIN_EXT) ./cmd/netsapiens-mcp

install-mcp:
	go install ./cmd/netsapiens-mcp

build-all: build build-mcp
