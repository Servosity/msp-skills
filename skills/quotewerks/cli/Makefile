.PHONY: build test lint install clean

BIN_EXT := $(if $(filter windows,$(shell go env GOOS)),.exe,)

build:
	go build -trimpath -o bin/quotewerks-cli$(BIN_EXT) ./cmd/quotewerks-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/quotewerks-cli

clean:
	rm -rf bin/

build-mcp:
	go build -trimpath -o bin/quotewerks-mcp$(BIN_EXT) ./cmd/quotewerks-mcp

install-mcp:
	go install ./cmd/quotewerks-mcp

build-all: build build-mcp
