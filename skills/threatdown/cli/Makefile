.PHONY: build test lint install clean

BIN_EXT := $(if $(filter windows,$(shell go env GOOS)),.exe,)

build:
	go build -trimpath -o bin/threatdown-cli$(BIN_EXT) ./cmd/threatdown-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/threatdown-cli

clean:
	rm -rf bin/

build-mcp:
	go build -trimpath -o bin/threatdown-mcp$(BIN_EXT) ./cmd/threatdown-mcp

install-mcp:
	go install ./cmd/threatdown-mcp

build-all: build build-mcp
