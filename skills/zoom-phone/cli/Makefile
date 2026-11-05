.PHONY: build test lint install clean

BIN_EXT := $(if $(filter windows,$(shell go env GOOS)),.exe,)

build:
	go build -trimpath -o bin/zoom-phone-cli$(BIN_EXT) ./cmd/zoom-phone-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/zoom-phone-cli

clean:
	rm -rf bin/

build-mcp:
	go build -trimpath -o bin/zoom-phone-mcp$(BIN_EXT) ./cmd/zoom-phone-mcp

install-mcp:
	go install ./cmd/zoom-phone-mcp

build-all: build build-mcp
