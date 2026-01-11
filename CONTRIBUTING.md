# Contributing to Resolute

Thank you for your interest in contributing to Resolute!

## Development Setup

1. **Clone the repository**
   ```bash
   git clone https://github.com/resolute-sh/resolute.git
   cd resolute
   ```

2. **Install Go 1.22+**
   ```bash
   # macOS
   brew install go

   # Or download from https://go.dev/dl/
   ```

3. **Install development tools**
   ```bash
   # golangci-lint for linting
   brew install golangci-lint

   # Or: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
   ```

4. **Verify setup**
   ```bash
   make test
   ```

## Development Workflow

### Running Tests

```bash
# Run all tests
make test

# Run core tests only
make test-core

# Run provider tests only
make test-providers

# Run with coverage
make coverage
```

### Code Quality

```bash
# Run linter
make lint

# Format code
make fmt

# Run go vet
make vet
```

### Building

```bash
# Build all modules
make build

# Verify dependencies
make verify
```

## Pull Request Process

1. **Fork the repository** and create your branch from `main`

2. **Make your changes**
   - Write or update tests as needed
   - Follow existing code style and patterns
   - Update documentation if applicable

3. **Ensure quality checks pass**
   ```bash
   make lint test
   ```

4. **Write a clear commit message**
   - Use present tense ("Add feature" not "Added feature")
   - Reference issues if applicable ("Fix #123: ...")

5. **Submit a pull request**
   - Describe what changes you made and why
   - Link to any related issues

## Code Style

- Follow standard Go conventions (`gofmt`, `go vet`)
- Use meaningful variable and function names
- Add comments for exported functions and types
- Keep functions focused and small
- Write table-driven tests with `t.Parallel()`

## Project Structure

```
resolute/
├── core/           # Core framework (flows, nodes, state, etc.)
├── state/          # State persistence backends
└── internal/       # Internal packages

resolute-{provider}/  # Provider packages (jira, qdrant, etc.)
├── provider.go     # Provider implementation
├── client.go       # API client
└── *_test.go       # Tests

resolute-docs/      # Documentation site
```

## Adding a New Provider

1. Create a new module: `resolute-{name}/`
2. Implement the `core.Provider` interface
3. Add typed input/output structs
4. Create helper functions returning `*core.Node[I, O]`
5. Write comprehensive tests
6. Add documentation

## Questions?

- Open an issue for bugs or feature requests
- Check existing issues before creating new ones

Thank you for contributing!
