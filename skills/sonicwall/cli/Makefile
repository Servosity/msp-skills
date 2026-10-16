.PHONY: build test lint install clean

BIN_EXT := $(if $(filter windows,$(shell go env GOOS)),.exe,)

build:
	go build -trimpath -o bin/sonicwall-cli$(BIN_EXT) ./cmd/sonicwall-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/sonicwall-cli

clean:
	rm -rf bin/

build-mcp:
	go build -trimpath -o bin/sonicwall-mcp$(BIN_EXT) ./cmd/sonicwall-mcp

install-mcp:
	go install ./cmd/sonicwall-mcp

build-all: build build-mcp
