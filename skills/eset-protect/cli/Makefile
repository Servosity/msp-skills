.PHONY: build test lint install clean

BIN_EXT := $(if $(filter windows,$(shell go env GOOS)),.exe,)

build:
	go build -trimpath -o bin/eset-protect-cli$(BIN_EXT) ./cmd/eset-protect-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/eset-protect-cli

clean:
	rm -rf bin/

build-mcp:
	go build -trimpath -o bin/eset-protect-mcp$(BIN_EXT) ./cmd/eset-protect-mcp

install-mcp:
	go install ./cmd/eset-protect-mcp

build-all: build build-mcp
