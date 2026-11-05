.PHONY: build test lint install clean

BIN_EXT := $(if $(filter windows,$(shell go env GOOS)),.exe,)

build:
	go build -trimpath -o bin/superops-cli$(BIN_EXT) ./cmd/superops-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/superops-cli

clean:
	rm -rf bin/

build-mcp:
	go build -trimpath -o bin/superops-mcp$(BIN_EXT) ./cmd/superops-mcp

install-mcp:
	go install ./cmd/superops-mcp

build-all: build build-mcp
