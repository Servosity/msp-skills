.PHONY: build test lint install clean

build:
	go build -o bin/aws-billing-cli ./cmd/aws-billing-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/aws-billing-cli

clean:
	rm -rf bin/

build-mcp:
	go build -o bin/aws-billing-mcp ./cmd/aws-billing-mcp

install-mcp:
	go install ./cmd/aws-billing-mcp

build-all: build build-mcp
