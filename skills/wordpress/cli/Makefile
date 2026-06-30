.PHONY: build test lint install clean

BIN_EXT := $(if $(filter windows,$(shell go env GOOS)),.exe,)

build:
	go build -o bin/wordpress-cli$(BIN_EXT) ./cmd/wordpress-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/wordpress-cli

clean:
	rm -rf bin/

build-mcp:
	go build -o bin/wordpress-mcp$(BIN_EXT) ./cmd/wordpress-mcp

install-mcp:
	go install ./cmd/wordpress-mcp

build-all: build build-mcp
