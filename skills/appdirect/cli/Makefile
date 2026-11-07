.PHONY: build test lint install clean

BIN_EXT := $(if $(filter windows,$(shell go env GOOS)),.exe,)

build:
	go build -trimpath -o bin/appdirect-cli$(BIN_EXT) ./cmd/appdirect-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/appdirect-cli

clean:
	rm -rf bin/

build-mcp:
	go build -trimpath -o bin/appdirect-mcp$(BIN_EXT) ./cmd/appdirect-mcp

install-mcp:
	go install ./cmd/appdirect-mcp

build-all: build build-mcp
