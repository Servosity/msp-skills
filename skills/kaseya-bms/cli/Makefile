.PHONY: build test lint install clean

BIN_EXT := $(if $(filter windows,$(shell go env GOOS)),.exe,)

build:
	go build -trimpath -o bin/kaseya-bms-cli$(BIN_EXT) ./cmd/kaseya-bms-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/kaseya-bms-cli

clean:
	rm -rf bin/

build-mcp:
	go build -trimpath -o bin/kaseya-bms-mcp$(BIN_EXT) ./cmd/kaseya-bms-mcp

install-mcp:
	go install ./cmd/kaseya-bms-mcp

build-all: build build-mcp
