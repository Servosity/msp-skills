.PHONY: build test lint install clean

BIN_EXT := $(if $(filter windows,$(shell go env GOOS)),.exe,)

build:
	go build -o bin/immybot-cli$(BIN_EXT) ./cmd/immybot-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/immybot-cli

clean:
	rm -rf bin/

build-mcp:
	go build -o bin/immybot-mcp$(BIN_EXT) ./cmd/immybot-mcp

install-mcp:
	go install ./cmd/immybot-mcp

build-all: build build-mcp
