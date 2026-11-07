.PHONY: build test lint install clean

BIN_EXT := $(if $(filter windows,$(shell go env GOOS)),.exe,)

build:
	go build -trimpath -o bin/action1-cli$(BIN_EXT) ./cmd/action1-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/action1-cli

clean:
	rm -rf bin/

build-mcp:
	go build -trimpath -o bin/action1-mcp$(BIN_EXT) ./cmd/action1-mcp

install-mcp:
	go install ./cmd/action1-mcp

build-all: build build-mcp
