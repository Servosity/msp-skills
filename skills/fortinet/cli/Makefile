.PHONY: build test lint install clean

BIN_EXT := $(if $(filter windows,$(shell go env GOOS)),.exe,)

build:
	go build -trimpath -o bin/fortinet-cli$(BIN_EXT) ./cmd/fortinet-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/fortinet-cli

clean:
	rm -rf bin/

build-mcp:
	go build -trimpath -o bin/fortinet-mcp$(BIN_EXT) ./cmd/fortinet-mcp

install-mcp:
	go install ./cmd/fortinet-mcp

build-all: build build-mcp
