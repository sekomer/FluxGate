.PHONY: all build test clean run fmt vet lint install docker

# Variables
BINARY_NAME=fluxgate
DOCKER_IMAGE=fluxgate:latest
GO_FILES=$(shell find . -name '*.go' -type f)

# Default target
all: test build

# Build the binary
build:
	go build -o $(BINARY_NAME) cmd/fluxgate/main.go

# Run tests
test:
	go test -v ./...

# Run tests with race detector
test-race:
	go test -race -v ./...

# Run tests with coverage
test-coverage:
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

# Clean build artifacts
clean:
	go clean
	rm -f $(BINARY_NAME)
	rm -f coverage.out coverage.html

# Run the application
run: build
	./$(BINARY_NAME)

# Format code
fmt:
	go fmt ./...

# Run go vet
vet:
	go vet ./...

# Run golangci-lint (requires golangci-lint installed)
lint:
	golangci-lint run

# Install the binary
install: build
	go install cmd/fluxgate/main.go

# Build Docker image
docker:
	docker build -t $(DOCKER_IMAGE) .

# Run with example config
demo: build
	./$(BINARY_NAME) -config examples/fluxgate.yaml

# Generate TLS certificates for testing
certs:
	cd examples && ./generate-certs.sh

# Run benchmarks
bench:
	go test -bench=. -benchmem ./...

# Update dependencies
deps:
	go mod tidy
	go mod vendor

# Check for security vulnerabilities
security:
	go list -json -m all | nancy sleuth

# Generate documentation
docs:
	godoc -http=:6060