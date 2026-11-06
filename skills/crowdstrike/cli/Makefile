.PHONY: build test lint install clean

BIN_EXT := $(if $(filter windows,$(shell go env GOOS)),.exe,)

build:
	go build -trimpath -o bin/crowdstrike-cli$(BIN_EXT) ./cmd/crowdstrike-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/crowdstrike-cli

clean:
	rm -rf bin/

build-mcp:
	go build -trimpath -o bin/crowdstrike-mcp$(BIN_EXT) ./cmd/crowdstrike-mcp

install-mcp:
	go install ./cmd/crowdstrike-mcp

build-all: build build-mcp
