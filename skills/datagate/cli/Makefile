.PHONY: build test lint install clean

BIN_EXT := $(if $(filter windows,$(shell go env GOOS)),.exe,)

build:
	go build -o bin/datagate-cli$(BIN_EXT) ./cmd/datagate-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/datagate-cli

clean:
	rm -rf bin/

build-mcp:
	go build -o bin/datagate-mcp$(BIN_EXT) ./cmd/datagate-mcp

install-mcp:
	go install ./cmd/datagate-mcp

build-all: build build-mcp
