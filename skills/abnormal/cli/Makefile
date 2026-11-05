.PHONY: build test lint install clean

BIN_EXT := $(if $(filter windows,$(shell go env GOOS)),.exe,)

build:
	go build -trimpath -o bin/abnormal-cli$(BIN_EXT) ./cmd/abnormal-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/abnormal-cli

clean:
	rm -rf bin/

build-mcp:
	go build -trimpath -o bin/abnormal-mcp$(BIN_EXT) ./cmd/abnormal-mcp

install-mcp:
	go install ./cmd/abnormal-mcp

build-all: build build-mcp
