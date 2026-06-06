.PHONY: build test lint install clean

build:
	go build -o bin/crowdstrike-cli ./cmd/crowdstrike-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/crowdstrike-cli

clean:
	rm -rf bin/

build-mcp:
	go build -o bin/crowdstrike-mcp ./cmd/crowdstrike-mcp

install-mcp:
	go install ./cmd/crowdstrike-mcp

build-all: build build-mcp
