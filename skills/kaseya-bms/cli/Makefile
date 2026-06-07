.PHONY: build test lint install clean

build:
	go build -o bin/kaseya-bms-cli ./cmd/kaseya-bms-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/kaseya-bms-cli

clean:
	rm -rf bin/

build-mcp:
	go build -o bin/kaseya-bms-mcp ./cmd/kaseya-bms-mcp

install-mcp:
	go install ./cmd/kaseya-bms-mcp

build-all: build build-mcp
