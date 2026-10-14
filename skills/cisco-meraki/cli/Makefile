.PHONY: build test lint install clean

BIN_EXT := $(if $(filter windows,$(shell go env GOOS)),.exe,)

build:
	go build -trimpath -o bin/cisco-meraki-cli$(BIN_EXT) ./cmd/cisco-meraki-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/cisco-meraki-cli

clean:
	rm -rf bin/

build-mcp:
	go build -trimpath -o bin/cisco-meraki-mcp$(BIN_EXT) ./cmd/cisco-meraki-mcp

install-mcp:
	go install ./cmd/cisco-meraki-mcp

build-all: build build-mcp
