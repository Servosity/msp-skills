.PHONY: build test lint install clean

BIN_EXT := $(if $(filter windows,$(shell go env GOOS)),.exe,)

build:
	go build -trimpath -o bin/cisco-duo-cli$(BIN_EXT) ./cmd/cisco-duo-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/cisco-duo-cli

clean:
	rm -rf bin/

build-mcp:
	go build -trimpath -o bin/cisco-duo-mcp$(BIN_EXT) ./cmd/cisco-duo-mcp

install-mcp:
	go install ./cmd/cisco-duo-mcp

build-all: build build-mcp
