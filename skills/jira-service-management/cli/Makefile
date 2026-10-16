.PHONY: build test lint install clean

BIN_EXT := $(if $(filter windows,$(shell go env GOOS)),.exe,)

build:
	go build -trimpath -o bin/jira-service-management-cli$(BIN_EXT) ./cmd/jira-service-management-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/jira-service-management-cli

clean:
	rm -rf bin/

build-mcp:
	go build -trimpath -o bin/jira-service-management-mcp$(BIN_EXT) ./cmd/jira-service-management-mcp

install-mcp:
	go install ./cmd/jira-service-management-mcp

build-all: build build-mcp
