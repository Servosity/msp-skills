.PHONY: build test lint install clean

BIN_EXT := $(if $(filter windows,$(shell go env GOOS)),.exe,)

build:
	go build -trimpath -o bin/maxio-cli$(BIN_EXT) ./cmd/maxio-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/maxio-cli

clean:
	rm -rf bin/

build-mcp:
	go build -trimpath -o bin/maxio-mcp$(BIN_EXT) ./cmd/maxio-mcp

install-mcp:
	go install ./cmd/maxio-mcp

build-all: build build-mcp
