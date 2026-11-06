.PHONY: build test lint install clean

BIN_EXT := $(if $(filter windows,$(shell go env GOOS)),.exe,)

build:
	go build -trimpath -o bin/afi-cli$(BIN_EXT) ./cmd/afi-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/afi-cli

clean:
	rm -rf bin/

build-mcp:
	go build -trimpath -o bin/afi-mcp$(BIN_EXT) ./cmd/afi-mcp

install-mcp:
	go install ./cmd/afi-mcp

build-all: build build-mcp
