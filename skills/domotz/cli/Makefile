.PHONY: build test lint install clean

build:
	go build -o bin/domotz-cli ./cmd/domotz-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/domotz-cli

clean:
	rm -rf bin/

build-mcp:
	go build -o bin/domotz-mcp ./cmd/domotz-mcp

install-mcp:
	go install ./cmd/domotz-mcp

build-all: build build-mcp
