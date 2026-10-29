.PHONY: build test lint install clean

BIN_EXT := $(if $(filter windows,$(shell go env GOOS)),.exe,)

build:
	go build -trimpath -o bin/cisco-umbrella-cli$(BIN_EXT) ./cmd/cisco-umbrella-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/cisco-umbrella-cli

clean:
	rm -rf bin/

build-mcp:
	go build -trimpath -o bin/cisco-umbrella-mcp$(BIN_EXT) ./cmd/cisco-umbrella-mcp

install-mcp:
	go install ./cmd/cisco-umbrella-mcp

build-all: build build-mcp
