.PHONY: build test lint install clean

BIN_EXT := $(if $(filter windows,$(shell go env GOOS)),.exe,)

build:
	go build -o bin/immybot-pp-cli$(BIN_EXT) ./cmd/immybot-pp-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/immybot-pp-cli

clean:
	rm -rf bin/

build-mcp:
	go build -o bin/immybot-pp-mcp$(BIN_EXT) ./cmd/immybot-pp-mcp

install-mcp:
	go install ./cmd/immybot-pp-mcp

build-all: build build-mcp
