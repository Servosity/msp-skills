.PHONY: build test lint install clean

BIN_EXT := $(if $(filter windows,$(shell go env GOOS)),.exe,)

build:
	go build -trimpath -o bin/proxmox-cli$(BIN_EXT) ./cmd/proxmox-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/proxmox-cli

clean:
	rm -rf bin/

build-mcp:
	go build -trimpath -o bin/proxmox-mcp$(BIN_EXT) ./cmd/proxmox-mcp

install-mcp:
	go install ./cmd/proxmox-mcp

build-all: build build-mcp
