.PHONY: build test lint install clean

BIN_EXT := $(if $(filter windows,$(shell go env GOOS)),.exe,)

build:
	go build -trimpath -o bin/rocketcyber-cli$(BIN_EXT) ./cmd/rocketcyber-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/rocketcyber-cli

clean:
	rm -rf bin/

build-mcp:
	go build -trimpath -o bin/rocketcyber-mcp$(BIN_EXT) ./cmd/rocketcyber-mcp

install-mcp:
	go install ./cmd/rocketcyber-mcp

build-all: build build-mcp
