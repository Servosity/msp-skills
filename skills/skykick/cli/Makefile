.PHONY: build test lint install clean

build:
	go build -o bin/skykick-cli ./cmd/skykick-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/skykick-cli

clean:
	rm -rf bin/

build-mcp:
	go build -o bin/skykick-mcp ./cmd/skykick-mcp

install-mcp:
	go install ./cmd/skykick-mcp

build-all: build build-mcp
