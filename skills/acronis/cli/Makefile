.PHONY: build test lint install clean

BIN_EXT := $(if $(filter windows,$(shell go env GOOS)),.exe,)

build:
	go build -trimpath -o bin/acronis-cli$(BIN_EXT) ./cmd/acronis-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/acronis-cli

clean:
	rm -rf bin/

build-mcp:
	go build -trimpath -o bin/acronis-mcp$(BIN_EXT) ./cmd/acronis-mcp

install-mcp:
	go install ./cmd/acronis-mcp

build-all: build build-mcp
