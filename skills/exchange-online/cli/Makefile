.PHONY: build test lint install clean

BIN_EXT := $(if $(filter windows,$(shell go env GOOS)),.exe,)

build:
	go build -trimpath -o bin/exchange-online-cli$(BIN_EXT) ./cmd/exchange-online-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/exchange-online-cli

clean:
	rm -rf bin/

build-mcp:
	go build -trimpath -o bin/exchange-online-mcp$(BIN_EXT) ./cmd/exchange-online-mcp

install-mcp:
	go install ./cmd/exchange-online-mcp

build-all: build build-mcp
