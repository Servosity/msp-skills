.PHONY: build test lint install clean

BIN_EXT := $(if $(filter windows,$(shell go env GOOS)),.exe,)

build:
	go build -trimpath -o bin/rewst-cli$(BIN_EXT) ./cmd/rewst-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/rewst-cli

clean:
	rm -rf bin/

build-mcp:
	go build -trimpath -o bin/rewst-mcp$(BIN_EXT) ./cmd/rewst-mcp

install-mcp:
	go install ./cmd/rewst-mcp

build-all: build build-mcp
