.PHONY: build test lint install clean

BIN_EXT := $(if $(filter windows,$(shell go env GOOS)),.exe,)

build:
	go build -trimpath -o bin/mimecast-cli$(BIN_EXT) ./cmd/mimecast-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/mimecast-cli

clean:
	rm -rf bin/

build-mcp:
	go build -trimpath -o bin/mimecast-mcp$(BIN_EXT) ./cmd/mimecast-mcp

install-mcp:
	go install ./cmd/mimecast-mcp

build-all: build build-mcp
