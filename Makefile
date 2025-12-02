BINARY_NAME=envir
VERSION := $(shell git describe --tags --always 2>/dev/null || echo "dev")
BUILD_DIR=build
DIST_DIR=dist

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOMOD=$(GOCMD) mod

# Build flags
LDFLAGS=-ldflags "-s -w -X github.com/yejune/go-envir/cmd.Version=$(VERSION)"

.PHONY: all build build-cross clean test deps install uninstall darwin linux help

all: deps build

deps:
	$(GOMOD) tidy

build: darwin linux

build-cross:
	@mkdir -p $(DIST_DIR)
	GOOS=darwin GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(DIST_DIR)/$(BINARY_NAME)-darwin-amd64 .
	GOOS=darwin GOARCH=arm64 $(GOBUILD) $(LDFLAGS) -o $(DIST_DIR)/$(BINARY_NAME)-darwin-arm64 .
	GOOS=linux GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(DIST_DIR)/$(BINARY_NAME)-linux-amd64 .
	GOOS=linux GOARCH=arm64 $(GOBUILD) $(LDFLAGS) -o $(DIST_DIR)/$(BINARY_NAME)-linux-arm64 .

darwin:
	@mkdir -p $(BUILD_DIR)
	@echo "Building for macOS (darwin/amd64)..."
	GOOS=darwin GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-amd64 .
	@echo "Building for macOS (darwin/arm64)..."
	GOOS=darwin GOARCH=arm64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-arm64 .

linux:
	@mkdir -p $(BUILD_DIR)
	@echo "Building for Linux (linux/amd64)..."
	GOOS=linux GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 .
	@echo "Building for Linux (linux/arm64)..."
	GOOS=linux GOARCH=arm64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-arm64 .

# Build for current platform only
local:
	@echo "Building for current platform..."
	$(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) .

clean:
	$(GOCLEAN)
	rm -rf $(BUILD_DIR) $(DIST_DIR)

test:
	$(GOTEST) -v ./...

# Install to /usr/local/bin
install: local
	@echo "Installing to /usr/local/bin..."
	@sudo cp $(BUILD_DIR)/$(BINARY_NAME) /usr/local/bin/$(BINARY_NAME)
	@sudo chmod +x /usr/local/bin/$(BINARY_NAME)
	@echo "✓ Installed to /usr/local/bin/$(BINARY_NAME)"

uninstall:
	@echo "Uninstalling..."
	@sudo rm -f /usr/local/bin/$(BINARY_NAME)
	@echo "Uninstalled"

# Show help
help:
	@echo "Go Envir - SSH deployment tool"
	@echo ""
	@echo "Usage:"
	@echo "  make deps      - Download dependencies"
	@echo "  make build     - Build for all platforms (darwin/linux)"
	@echo "  make darwin    - Build for macOS only"
	@echo "  make linux     - Build for Linux only"
	@echo "  make local     - Build for current platform"
	@echo "  make install   - Install to /usr/local/bin"
	@echo "  make uninstall - Remove from /usr/local/bin"
	@echo "  make clean     - Clean build artifacts"
	@echo "  make test      - Run tests"
