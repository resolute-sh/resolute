# Contributing to Resolute

## About This Project

Resolute is currently a **solo builder's project**. The primary goal is to create an AI agent framework that **just works out of the box**—no complex setup, no configuration hell, just workflows that run.

This means development follows a focused roadmap rather than being driven by external contributions. That said, the project is open source and contributions are welcome within certain constraints.

## Before You Contribute

**Please reach out before starting work on a PR.** Given bandwidth constraints, it's better to discuss whether a contribution aligns with the current roadmap before you invest time. Open an issue or discussion first.

Contributions most likely to be integrated:

- Bug fixes with minimal footprint
- Documentation improvements
- Test coverage improvements
- Small, focused changes that don't alter core architecture

Contributions that need discussion first:

- New features or nodes
- Changes to core abstractions (flows, state, execution model)
- New provider integrations
- Anything that increases API surface area

## Current Focus

The current priority is making Resolute a **batteries-included framework** for building AI agents:

1. Core primitives that compose well
2. State management that's invisible when you don't need it, powerful when you do
3. Providers that just work
4. Clear, practical documentation

Contributions aligned with these goals have a better chance of being reviewed and merged.

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
   brew install golangci-lint
   ```

4. **Verify setup**
   ```bash
   make test
   ```

## Development Workflow

```bash
# Run all tests
make test

# Run linter
make lint

# Format code
make fmt

# Build all modules
make build
```

## Code Style

- Standard Go conventions (`gofmt`, `go vet`)
- Meaningful names over comments
- Small, focused functions
- Table-driven tests with `t.Parallel()`

## Pull Request Process

1. **Discuss first** - Open an issue or discussion
2. **Keep it small** - Focused changes are easier to review
3. **Tests pass** - `make lint test` must succeed
4. **Clear description** - Explain the what and why

## Questions?

Open an issue. Bug reports and questions are always welcome, even if PR bandwidth is limited.
