.PHONY: build test lint install clean

build:
	go build -o bin/abnormal-cli ./cmd/abnormal-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/abnormal-cli

clean:
	rm -rf bin/

build-mcp:
	go build -o bin/abnormal-mcp ./cmd/abnormal-mcp

install-mcp:
	go install ./cmd/abnormal-mcp

build-all: build build-mcp
