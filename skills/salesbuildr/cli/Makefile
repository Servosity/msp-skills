.PHONY: build test lint install clean

BIN_EXT := $(if $(filter windows,$(shell go env GOOS)),.exe,)

build:
	go build -trimpath -o bin/salesbuildr-cli$(BIN_EXT) ./cmd/salesbuildr-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/salesbuildr-cli

clean:
	rm -rf bin/

build-mcp:
	go build -trimpath -o bin/salesbuildr-mcp$(BIN_EXT) ./cmd/salesbuildr-mcp

install-mcp:
	go install ./cmd/salesbuildr-mcp

build-all: build build-mcp
