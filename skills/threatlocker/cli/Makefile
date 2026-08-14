.PHONY: build test lint install clean

BIN_EXT := $(if $(filter windows,$(shell go env GOOS)),.exe,)

build:
	go build -o bin/threatlocker-cli$(BIN_EXT) ./cmd/threatlocker-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/threatlocker-cli

clean:
	rm -rf bin/

build-mcp:
	go build -o bin/threatlocker-mcp$(BIN_EXT) ./cmd/threatlocker-mcp

install-mcp:
	go install ./cmd/threatlocker-mcp

build-all: build build-mcp
