.PHONY: build test lint install clean

BIN_EXT := $(if $(filter windows,$(shell go env GOOS)),.exe,)

build:
	go build -trimpath -o bin/veeam-cli$(BIN_EXT) ./cmd/veeam-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/veeam-cli

clean:
	rm -rf bin/

build-mcp:
	go build -trimpath -o bin/veeam-mcp$(BIN_EXT) ./cmd/veeam-mcp

install-mcp:
	go install ./cmd/veeam-mcp

build-all: build build-mcp
