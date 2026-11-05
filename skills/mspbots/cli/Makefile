.PHONY: build test lint install clean

BIN_EXT := $(if $(filter windows,$(shell go env GOOS)),.exe,)

build:
	go build -trimpath -o bin/mspbots-cli$(BIN_EXT) ./cmd/mspbots-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/mspbots-cli

clean:
	rm -rf bin/

build-mcp:
	go build -trimpath -o bin/mspbots-mcp$(BIN_EXT) ./cmd/mspbots-mcp

install-mcp:
	go install ./cmd/mspbots-mcp

build-all: build build-mcp
