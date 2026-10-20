.PHONY: build test lint install clean

BIN_EXT := $(if $(filter windows,$(shell go env GOOS)),.exe,)

build:
	go build -trimpath -o bin/td-synnex-cli$(BIN_EXT) ./cmd/td-synnex-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/td-synnex-cli

clean:
	rm -rf bin/

build-mcp:
	go build -trimpath -o bin/td-synnex-mcp$(BIN_EXT) ./cmd/td-synnex-mcp

install-mcp:
	go install ./cmd/td-synnex-mcp

build-all: build build-mcp
