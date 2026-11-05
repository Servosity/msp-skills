.PHONY: build test lint install clean

BIN_EXT := $(if $(filter windows,$(shell go env GOOS)),.exe,)

build:
	go build -trimpath -o bin/connectwise-control-cli$(BIN_EXT) ./cmd/connectwise-control-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/connectwise-control-cli

clean:
	rm -rf bin/

build-mcp:
	go build -trimpath -o bin/connectwise-control-mcp$(BIN_EXT) ./cmd/connectwise-control-mcp

install-mcp:
	go install ./cmd/connectwise-control-mcp

build-all: build build-mcp
