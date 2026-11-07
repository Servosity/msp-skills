.PHONY: build test lint install clean

BIN_EXT := $(if $(filter windows,$(shell go env GOOS)),.exe,)

build:
	go build -trimpath -o bin/keeper-cli$(BIN_EXT) ./cmd/keeper-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/keeper-cli

clean:
	rm -rf bin/

build-mcp:
	go build -trimpath -o bin/keeper-mcp$(BIN_EXT) ./cmd/keeper-mcp

install-mcp:
	go install ./cmd/keeper-mcp

build-all: build build-mcp
