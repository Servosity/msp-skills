.PHONY: build test lint install clean

BIN_EXT := $(if $(filter windows,$(shell go env GOOS)),.exe,)

build:
	go build -trimpath -o bin/knowbe4-cli$(BIN_EXT) ./cmd/knowbe4-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/knowbe4-cli

clean:
	rm -rf bin/

build-mcp:
	go build -trimpath -o bin/knowbe4-mcp$(BIN_EXT) ./cmd/knowbe4-mcp

install-mcp:
	go install ./cmd/knowbe4-mcp

build-all: build build-mcp
