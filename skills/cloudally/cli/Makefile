.PHONY: build test lint install clean

BIN_EXT := $(if $(filter windows,$(shell go env GOOS)),.exe,)

build:
	go build -trimpath -o bin/cloudally-cli$(BIN_EXT) ./cmd/cloudally-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/cloudally-cli

clean:
	rm -rf bin/

build-mcp:
	go build -trimpath -o bin/cloudally-mcp$(BIN_EXT) ./cmd/cloudally-mcp

install-mcp:
	go install ./cmd/cloudally-mcp

build-all: build build-mcp
