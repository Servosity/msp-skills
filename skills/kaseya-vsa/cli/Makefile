.PHONY: build test lint install clean

BIN_EXT := $(if $(filter windows,$(shell go env GOOS)),.exe,)

build:
	go build -trimpath -o bin/kaseya-vsa-cli$(BIN_EXT) ./cmd/kaseya-vsa-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/kaseya-vsa-cli

clean:
	rm -rf bin/

build-mcp:
	go build -trimpath -o bin/kaseya-vsa-mcp$(BIN_EXT) ./cmd/kaseya-vsa-mcp

install-mcp:
	go install ./cmd/kaseya-vsa-mcp

build-all: build build-mcp
