.PHONY: build test lint install clean

BIN_EXT := $(if $(filter windows,$(shell go env GOOS)),.exe,)

build:
	go build -trimpath -o bin/pagerduty-cli$(BIN_EXT) ./cmd/pagerduty-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/pagerduty-cli

clean:
	rm -rf bin/

build-mcp:
	go build -trimpath -o bin/pagerduty-mcp$(BIN_EXT) ./cmd/pagerduty-mcp

install-mcp:
	go install ./cmd/pagerduty-mcp

build-all: build build-mcp
