.PHONY: build test lint install clean

BIN_EXT := $(if $(filter windows,$(shell go env GOOS)),.exe,)

build:
	go build -o bin/cork-cli$(BIN_EXT) ./cmd/cork-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/cork-cli

clean:
	rm -rf bin/

build-mcp:
	go build -o bin/cork-mcp$(BIN_EXT) ./cmd/cork-mcp

install-mcp:
	go install ./cmd/cork-mcp

build-all: build build-mcp
