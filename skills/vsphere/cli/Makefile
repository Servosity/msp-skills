.PHONY: build test lint install clean

BIN_EXT := $(if $(filter windows,$(shell go env GOOS)),.exe,)

build:
	go build -trimpath -o bin/vsphere-cli$(BIN_EXT) ./cmd/vsphere-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/vsphere-cli

clean:
	rm -rf bin/

build-mcp:
	go build -trimpath -o bin/vsphere-mcp$(BIN_EXT) ./cmd/vsphere-mcp

install-mcp:
	go install ./cmd/vsphere-mcp

build-all: build build-mcp
