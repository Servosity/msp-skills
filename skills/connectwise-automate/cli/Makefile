.PHONY: build test lint install clean

build:
	go build -o bin/connectwise-automate-cli ./cmd/connectwise-automate-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/connectwise-automate-cli

clean:
	rm -rf bin/

build-mcp:
	go build -o bin/connectwise-automate-mcp ./cmd/connectwise-automate-mcp

install-mcp:
	go install ./cmd/connectwise-automate-mcp

build-all: build build-mcp
