# Contributing to FluxGate

## Development Process

1. Fork the repo and create your branch from `main`
2. If you've added code that should be tested, add tests
3. If you've changed APIs, update the documentation
4. Ensure the test suite passes
5. Test with dynamic service registration scenarios
6. Make sure your code follows the existing style
7. Issue that pull request!

## Development Setup

```bash
# Clone your fork
git clone https://github.com/yourusername/fluxgate.git
cd fluxgate

# Install dependencies
go mod download

# Build FluxGate
go build -o fluxgate cmd/fluxgate/main.go

# Run tests
go test ./...

# Lint code
go vet ./...
```
