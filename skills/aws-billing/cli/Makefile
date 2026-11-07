.PHONY: build test lint install clean

BIN_EXT := $(if $(filter windows,$(shell go env GOOS)),.exe,)

build:
	go build -trimpath -o bin/aws-billing-cli$(BIN_EXT) ./cmd/aws-billing-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/aws-billing-cli

clean:
	rm -rf bin/

build-mcp:
	go build -trimpath -o bin/aws-billing-mcp$(BIN_EXT) ./cmd/aws-billing-mcp

install-mcp:
	go install ./cmd/aws-billing-mcp

build-all: build build-mcp
