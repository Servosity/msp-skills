.PHONY: build test lint install clean

BIN_EXT := $(if $(filter windows,$(shell go env GOOS)),.exe,)

build:
	go build -trimpath -o bin/windows-server-cli$(BIN_EXT) ./cmd/windows-server-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/windows-server-cli

clean:
	rm -rf bin/

build-mcp:
	go build -trimpath -o bin/windows-server-mcp$(BIN_EXT) ./cmd/windows-server-mcp

install-mcp:
	go install ./cmd/windows-server-mcp

build-all: build build-mcp
