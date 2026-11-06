.PHONY: build test lint install clean

BIN_EXT := $(if $(filter windows,$(shell go env GOOS)),.exe,)

build:
	go build -trimpath -o bin/datto-saas-cli$(BIN_EXT) ./cmd/datto-saas-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/datto-saas-cli

clean:
	rm -rf bin/

build-mcp:
	go build -trimpath -o bin/datto-saas-mcp$(BIN_EXT) ./cmd/datto-saas-mcp

install-mcp:
	go install ./cmd/datto-saas-mcp

build-all: build build-mcp
