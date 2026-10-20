.PHONY: build test lint install clean

BIN_EXT := $(if $(filter windows,$(shell go env GOOS)),.exe,)

build:
	go build -trimpath -o bin/saas-alerts-cli$(BIN_EXT) ./cmd/saas-alerts-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/saas-alerts-cli

clean:
	rm -rf bin/

build-mcp:
	go build -trimpath -o bin/saas-alerts-mcp$(BIN_EXT) ./cmd/saas-alerts-mcp

install-mcp:
	go install ./cmd/saas-alerts-mcp

build-all: build build-mcp
