.PHONY: build test lint install clean

BIN_EXT := $(if $(filter windows,$(shell go env GOOS)),.exe,)

build:
	go build -trimpath -o bin/dnsfilter-cli$(BIN_EXT) ./cmd/dnsfilter-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/dnsfilter-cli

clean:
	rm -rf bin/

build-mcp:
	go build -trimpath -o bin/dnsfilter-mcp$(BIN_EXT) ./cmd/dnsfilter-mcp

install-mcp:
	go install ./cmd/dnsfilter-mcp

build-all: build build-mcp
