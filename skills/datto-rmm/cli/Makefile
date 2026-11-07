.PHONY: build test lint install clean

BIN_EXT := $(if $(filter windows,$(shell go env GOOS)),.exe,)

build:
	go build -trimpath -o bin/datto-rmm-cli$(BIN_EXT) ./cmd/datto-rmm-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/datto-rmm-cli

clean:
	rm -rf bin/

build-mcp:
	go build -trimpath -o bin/datto-rmm-mcp$(BIN_EXT) ./cmd/datto-rmm-mcp

install-mcp:
	go install ./cmd/datto-rmm-mcp

build-all: build build-mcp
