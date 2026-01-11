.PHONY: all test test-core test-providers lint fmt build clean coverage docs verify help

# Default target
all: lint test build

# Run all tests with race detector
test: test-core test-providers

# Test core module
test-core:
	@echo "==> Testing core..."
	@go test -race -v ./...

# Test all provider modules (sibling directories)
test-providers:
	@echo "==> Testing providers..."
	@for dir in ../resolute-*; do \
		if [ -d "$$dir" ] && [ -f "$$dir/go.mod" ]; then \
			echo "==> Testing $$(basename $$dir)..."; \
			(cd $$dir && go test -race -v ./...) || exit 1; \
		fi \
	done

# Run linter on all modules
lint:
	@echo "==> Linting core..."
	@golangci-lint run --timeout=5m
	@for dir in ../resolute-*; do \
		if [ -d "$$dir" ] && [ -f "$$dir/go.mod" ]; then \
			echo "==> Linting $$(basename $$dir)..."; \
			(cd $$dir && golangci-lint run --timeout=5m) || exit 1; \
		fi \
	done

# Format all Go code
fmt:
	@echo "==> Formatting code..."
	@gofmt -w -s .
	@for dir in ../resolute-*; do \
		if [ -d "$$dir" ] && [ -f "$$dir/go.mod" ]; then \
			(cd $$dir && gofmt -w -s .) || exit 1; \
		fi \
	done

# Build all modules
build:
	@echo "==> Building core..."
	@go build ./...
	@for dir in ../resolute-*; do \
		if [ -d "$$dir" ] && [ -f "$$dir/go.mod" ]; then \
			echo "==> Building $$(basename $$dir)..."; \
			(cd $$dir && go build ./...) || exit 1; \
		fi \
	done
	@echo "==> Build complete"

# Run tests with coverage report
coverage:
	@echo "==> Running tests with coverage..."
	@go test -coverprofile=coverage.out -covermode=atomic ./...
	@go tool cover -html=coverage.out -o coverage.html
	@echo "==> Coverage report: coverage.html"

# Start documentation server
docs:
	@echo "==> Starting documentation server..."
	@cd ../resolute-docs && npm start

# Clean build artifacts
clean:
	@echo "==> Cleaning..."
	@rm -f coverage.out coverage.html
	@find . -name "*.test" -delete
	@for dir in ../resolute-*; do \
		if [ -d "$$dir" ]; then \
			rm -f $$dir/coverage.out $$dir/coverage.html; \
		fi \
	done
	@echo "==> Clean complete"

# Verify all module dependencies
verify:
	@echo "==> Verifying dependencies..."
	@go mod verify
	@for dir in ../resolute-*; do \
		if [ -d "$$dir" ] && [ -f "$$dir/go.mod" ]; then \
			echo "==> Verifying $$(basename $$dir)..."; \
			(cd $$dir && go mod verify) || exit 1; \
		fi \
	done
	@echo "==> All dependencies verified"

# Update all module dependencies
update-deps:
	@echo "==> Updating dependencies..."
	@go get -u ./... && go mod tidy
	@for dir in ../resolute-*; do \
		if [ -d "$$dir" ] && [ -f "$$dir/go.mod" ]; then \
			echo "==> Updating $$(basename $$dir)..."; \
			(cd $$dir && go get -u ./... && go mod tidy) || exit 1; \
		fi \
	done
	@echo "==> Dependencies updated"

# Run go vet on all modules
vet:
	@echo "==> Running go vet..."
	@go vet ./...
	@for dir in ../resolute-*; do \
		if [ -d "$$dir" ] && [ -f "$$dir/go.mod" ]; then \
			(cd $$dir && go vet ./...) || exit 1; \
		fi \
	done

# Show help
help:
	@echo "Resolute Development Commands"
	@echo ""
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@echo "  all            Run lint, test, and build (default)"
	@echo "  test           Run all tests with race detector"
	@echo "  test-core      Run tests for core module only"
	@echo "  test-providers Run tests for all provider modules"
	@echo "  lint           Run golangci-lint on all modules"
	@echo "  fmt            Format all Go code"
	@echo "  build          Build all modules"
	@echo "  coverage       Generate coverage report (coverage.html)"
	@echo "  docs           Start documentation server"
	@echo "  clean          Remove build artifacts"
	@echo "  verify         Verify module dependencies"
	@echo "  update-deps    Update all dependencies"
	@echo "  vet            Run go vet on all modules"
	@echo "  help           Show this help"
