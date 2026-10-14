.PHONY: build test lint install clean

BIN_EXT := $(if $(filter windows,$(shell go env GOOS)),.exe,)

build:
	go build -trimpath -o bin/connectwise-rmm-cli$(BIN_EXT) ./cmd/connectwise-rmm-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/connectwise-rmm-cli

clean:
	rm -rf bin/

build-mcp:
	go build -trimpath -o bin/connectwise-rmm-mcp$(BIN_EXT) ./cmd/connectwise-rmm-mcp

install-mcp:
	go install ./cmd/connectwise-rmm-mcp

build-all: build build-mcp
