.PHONY: build test lint install clean

BIN_EXT := $(if $(filter windows,$(shell go env GOOS)),.exe,)

build:
	go build -trimpath -o bin/warranty-cli$(BIN_EXT) ./cmd/warranty-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/warranty-cli

clean:
	rm -rf bin/

build-mcp:
	go build -trimpath -o bin/warranty-mcp$(BIN_EXT) ./cmd/warranty-mcp

install-mcp:
	go install ./cmd/warranty-mcp

build-all: build build-mcp
