.PHONY: build test lint install clean

BIN_EXT := $(if $(filter windows,$(shell go env GOOS)),.exe,)

build:
	go build -trimpath -o bin/jamf-pro-cli$(BIN_EXT) ./cmd/jamf-pro-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/jamf-pro-cli

clean:
	rm -rf bin/

build-mcp:
	go build -trimpath -o bin/jamf-pro-mcp$(BIN_EXT) ./cmd/jamf-pro-mcp

install-mcp:
	go install ./cmd/jamf-pro-mcp

build-all: build build-mcp
