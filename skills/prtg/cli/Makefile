.PHONY: build test lint install clean

BIN_EXT := $(if $(filter windows,$(shell go env GOOS)),.exe,)

build:
	go build -trimpath -o bin/prtg-cli$(BIN_EXT) ./cmd/prtg-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/prtg-cli

clean:
	rm -rf bin/

build-mcp:
	go build -trimpath -o bin/prtg-mcp$(BIN_EXT) ./cmd/prtg-mcp

install-mcp:
	go install ./cmd/prtg-mcp

build-all: build build-mcp
