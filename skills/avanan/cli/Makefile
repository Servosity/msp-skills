.PHONY: build test lint install clean

BIN_EXT := $(if $(filter windows,$(shell go env GOOS)),.exe,)

build:
	go build -o bin/avanan-cli$(BIN_EXT) ./cmd/avanan-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/avanan-cli

clean:
	rm -rf bin/

build-mcp:
	go build -o bin/avanan-mcp$(BIN_EXT) ./cmd/avanan-mcp

install-mcp:
	go install ./cmd/avanan-mcp

build-all: build build-mcp
