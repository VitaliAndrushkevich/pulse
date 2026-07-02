.PHONY: build test lint clean

# Default target
build:
	go build -o bin/pulse-mcp ./cmd/pulse-mcp

test:
	go test ./...

# Requires golangci-lint to be installed:
#   https://golangci-lint.run/welcome/install/
lint:
	golangci-lint run ./...

clean:
	rm -rf bin/
