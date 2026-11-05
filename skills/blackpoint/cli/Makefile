.PHONY: build test lint install clean

BIN_EXT := $(if $(filter windows,$(shell go env GOOS)),.exe,)

build:
	go build -trimpath -o bin/blackpoint-cli$(BIN_EXT) ./cmd/blackpoint-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/blackpoint-cli

clean:
	rm -rf bin/

build-mcp:
	go build -trimpath -o bin/blackpoint-mcp$(BIN_EXT) ./cmd/blackpoint-mcp

install-mcp:
	go install ./cmd/blackpoint-mcp

build-all: build build-mcp
