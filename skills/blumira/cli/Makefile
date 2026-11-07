.PHONY: build test lint install clean

BIN_EXT := $(if $(filter windows,$(shell go env GOOS)),.exe,)

build:
	go build -trimpath -o bin/blumira-cli$(BIN_EXT) ./cmd/blumira-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/blumira-cli

clean:
	rm -rf bin/

build-mcp:
	go build -trimpath -o bin/blumira-mcp$(BIN_EXT) ./cmd/blumira-mcp

install-mcp:
	go install ./cmd/blumira-mcp

build-all: build build-mcp
