# 5G UDM Project Makefile
# Based on: docs/testing-strategy.md §7 (CI/CD Integration)

.PHONY: all build test test-integration lint vet coverage clean tidy

# Default target
all: tidy vet lint test build

# Build all services
build:
	CGO_ENABLED=0 go build ./...

# Run all unit tests
test:
	go test ./... -count=1 -timeout 60s -race

# Run integration tests against YugabyteDB
# Requires YUGABYTE_DSN or a local YugabyteDB on port 5433.
# Usage: make test-integration
#        YUGABYTE_DSN="postgres://user:pass@host:5433/db" make test-integration
test-integration:
	go test ./migrations/... -tags=integration -count=1 -timeout 5m -v

# Run tests with coverage
coverage:
	go test ./... -count=1 -timeout 60s -coverprofile=coverage.out
	go tool cover -func=coverage.out
	@echo ""
	@echo "Coverage report generated: coverage.out"

# Run linter
lint:
	golangci-lint run ./...

# Run go vet
vet:
	go vet ./...

# Tidy go modules
tidy:
	go mod tidy

# Clean build artifacts
clean:
	rm -f coverage.out
	go clean ./...
