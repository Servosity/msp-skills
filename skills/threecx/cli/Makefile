.PHONY: build test lint install clean

BIN_EXT := $(if $(filter windows,$(shell go env GOOS)),.exe,)

build:
	go build -trimpath -o bin/threecx-cli$(BIN_EXT) ./cmd/threecx-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/threecx-cli

clean:
	rm -rf bin/

build-mcp:
	go build -trimpath -o bin/threecx-mcp$(BIN_EXT) ./cmd/threecx-mcp

install-mcp:
	go install ./cmd/threecx-mcp

build-all: build build-mcp
