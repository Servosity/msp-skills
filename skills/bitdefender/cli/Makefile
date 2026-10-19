.PHONY: build test lint install clean

BIN_EXT := $(if $(filter windows,$(shell go env GOOS)),.exe,)

build:
	go build -trimpath -o bin/bitdefender-cli$(BIN_EXT) ./cmd/bitdefender-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/bitdefender-cli

clean:
	rm -rf bin/

build-mcp:
	go build -trimpath -o bin/bitdefender-mcp$(BIN_EXT) ./cmd/bitdefender-mcp

install-mcp:
	go install ./cmd/bitdefender-mcp

build-all: build build-mcp
