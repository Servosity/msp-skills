.PHONY: build test lint install clean

BIN_EXT := $(if $(filter windows,$(shell go env GOOS)),.exe,)

build:
	go build -trimpath -o bin/cipp-cli$(BIN_EXT) ./cmd/cipp-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/cipp-cli

clean:
	rm -rf bin/

build-mcp:
	go build -trimpath -o bin/cipp-mcp$(BIN_EXT) ./cmd/cipp-mcp

install-mcp:
	go install ./cmd/cipp-mcp

build-all: build build-mcp
