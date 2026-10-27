.PHONY: build test lint install clean

BIN_EXT := $(if $(filter windows,$(shell go env GOOS)),.exe,)

build:
	go build -trimpath -o bin/hudu-cli$(BIN_EXT) ./cmd/hudu-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/hudu-cli

clean:
	rm -rf bin/

build-mcp:
	go build -trimpath -o bin/hudu-mcp$(BIN_EXT) ./cmd/hudu-mcp

install-mcp:
	go install ./cmd/hudu-mcp

build-all: build build-mcp
