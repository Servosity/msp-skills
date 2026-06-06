.PHONY: build test lint install clean

build:
	go build -o bin/pagerduty-cli ./cmd/pagerduty-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/pagerduty-cli

clean:
	rm -rf bin/

build-mcp:
	go build -o bin/pagerduty-mcp ./cmd/pagerduty-mcp

install-mcp:
	go install ./cmd/pagerduty-mcp

build-all: build build-mcp
