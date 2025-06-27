.PHONY: build run clean test deps help

# Binary name
BINARY_NAME=applicationset-analyzer
BUILD_DIR=bin

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod

# Build the application
build:
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p $(BUILD_DIR)
	$(GOBUILD) -o $(BUILD_DIR)/$(BINARY_NAME) main.go
	@echo "Build complete: $(BUILD_DIR)/$(BINARY_NAME)"

# Run the application
run:
	@echo "Running ApplicationSet Analyzer..."
	$(GOCMD) run main.go

# Clean build artifacts
clean:
	@echo "Cleaning..."
	$(GOCLEAN)
	rm -rf $(BUILD_DIR)

# Download dependencies
deps:
	@echo "Downloading dependencies..."
	$(GOMOD) download
	$(GOMOD) tidy

# Run tests
test:
	@echo "Running tests..."
	$(GOTEST) -v ./...

# Install and run with k8sgpt
install: build
	@echo "Installing as k8sgpt custom analyzer..."
	@echo "Make sure the analyzer is running in another terminal before proceeding"
	@echo "Run: make run"
	@echo "Then in another terminal:"
	@echo "k8sgpt custom-analyzer add -n applicationset-analyzer"
	@echo "k8sgpt custom-analyzer list"
	@echo "k8sgpt analyze --custom-analysis"

# Help
help:
	@echo "Available targets:"
	@echo "  build    - Build the analyzer binary"
	@echo "  run      - Run the analyzer server"
	@echo "  clean    - Clean build artifacts"
	@echo "  deps     - Download and tidy dependencies"
	@echo "  test     - Run tests"
	@echo "  install  - Instructions for k8sgpt integration"
	@echo "  help     - Show this help message" 