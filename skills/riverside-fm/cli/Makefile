.PHONY: build test lint install clean

BIN_EXT := $(if $(filter windows,$(shell go env GOOS)),.exe,)

build:
	go build -trimpath -o bin/riverside-fm-cli$(BIN_EXT) ./cmd/riverside-fm-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/riverside-fm-cli

clean:
	rm -rf bin/

build-mcp:
	go build -trimpath -o bin/riverside-fm-mcp$(BIN_EXT) ./cmd/riverside-fm-mcp

install-mcp:
	go install ./cmd/riverside-fm-mcp

build-all: build build-mcp
