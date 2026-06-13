.PHONY: build test lint install clean

build:
	go build -o bin/rewst-cli ./cmd/rewst-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/rewst-cli

clean:
	rm -rf bin/

build-mcp:
	go build -o bin/rewst-mcp ./cmd/rewst-mcp

install-mcp:
	go install ./cmd/rewst-mcp

build-all: build build-mcp
