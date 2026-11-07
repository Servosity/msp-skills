.PHONY: build test lint install clean

BIN_EXT := $(if $(filter windows,$(shell go env GOOS)),.exe,)

build:
	go build -trimpath -o bin/connectwise-automate-cli$(BIN_EXT) ./cmd/connectwise-automate-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/connectwise-automate-cli

clean:
	rm -rf bin/

build-mcp:
	go build -trimpath -o bin/connectwise-automate-mcp$(BIN_EXT) ./cmd/connectwise-automate-mcp

install-mcp:
	go install ./cmd/connectwise-automate-mcp

build-all: build build-mcp
