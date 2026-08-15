.PHONY: build test lint install clean

BIN_EXT := $(if $(filter windows,$(shell go env GOOS)),.exe,)

build:
	go build -o bin/auvik-cli$(BIN_EXT) ./cmd/auvik-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/auvik-cli

clean:
	rm -rf bin/

build-mcp:
	go build -o bin/auvik-mcp$(BIN_EXT) ./cmd/auvik-mcp

install-mcp:
	go install ./cmd/auvik-mcp

build-all: build build-mcp
