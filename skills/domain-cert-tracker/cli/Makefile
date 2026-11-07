.PHONY: build test lint install clean

BIN_EXT := $(if $(filter windows,$(shell go env GOOS)),.exe,)

build:
	go build -trimpath -o bin/domain-cert-tracker-cli$(BIN_EXT) ./cmd/domain-cert-tracker-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/domain-cert-tracker-cli

clean:
	rm -rf bin/

build-mcp:
	go build -trimpath -o bin/domain-cert-tracker-mcp$(BIN_EXT) ./cmd/domain-cert-tracker-mcp

install-mcp:
	go install ./cmd/domain-cert-tracker-mcp

build-all: build build-mcp
