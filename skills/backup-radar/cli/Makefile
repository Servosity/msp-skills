.PHONY: build test lint install clean

BIN_EXT := $(if $(filter windows,$(shell go env GOOS)),.exe,)

build:
	go build -trimpath -o bin/backup-radar-cli$(BIN_EXT) ./cmd/backup-radar-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/backup-radar-cli

clean:
	rm -rf bin/

build-mcp:
	go build -trimpath -o bin/backup-radar-mcp$(BIN_EXT) ./cmd/backup-radar-mcp

install-mcp:
	go install ./cmd/backup-radar-mcp

build-all: build build-mcp
