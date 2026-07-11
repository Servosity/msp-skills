.PHONY: build test lint install clean

BIN_EXT := $(if $(filter windows,$(shell go env GOOS)),.exe,)

build:
	go build -o bin/zammad-cli$(BIN_EXT) ./cmd/zammad-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/zammad-cli

clean:
	rm -rf bin/

build-mcp:
	go build -o bin/zammad-mcp$(BIN_EXT) ./cmd/zammad-mcp

install-mcp:
	go install ./cmd/zammad-mcp

build-all: build build-mcp
