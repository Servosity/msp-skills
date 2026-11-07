.PHONY: build test lint install clean

BIN_EXT := $(if $(filter windows,$(shell go env GOOS)),.exe,)

build:
	go build -trimpath -o bin/domotz-cli$(BIN_EXT) ./cmd/domotz-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/domotz-cli

clean:
	rm -rf bin/

build-mcp:
	go build -trimpath -o bin/domotz-mcp$(BIN_EXT) ./cmd/domotz-mcp

install-mcp:
	go install ./cmd/domotz-mcp

build-all: build build-mcp
