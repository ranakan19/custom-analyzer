.PHONY: build test clean

# Build the analyzer binary
build:
	go build -o bin/applicationset-analyzer main.go

# Run tests
test:
	go test -v ./...

# Clean build artifacts
clean:
	rm -rf bin/

# Run the analyzer locally
run: build
	./bin/custom-analyzer

# Download dependencies
deps:
	go mod download

# Verify dependencies
verify:
	go mod verify

# Format code
fmt:
	go fmt ./...

# Run linter
lint:
	go vet ./...

# All targets for CI
ci: deps verify fmt lint test build 