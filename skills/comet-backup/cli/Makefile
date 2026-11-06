.PHONY: build test lint install clean

BIN_EXT := $(if $(filter windows,$(shell go env GOOS)),.exe,)

build:
	go build -trimpath -o bin/comet-backup-cli$(BIN_EXT) ./cmd/comet-backup-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/comet-backup-cli

clean:
	rm -rf bin/

build-mcp:
	go build -trimpath -o bin/comet-backup-mcp$(BIN_EXT) ./cmd/comet-backup-mcp

install-mcp:
	go install ./cmd/comet-backup-mcp

build-all: build build-mcp
