# Resolute


> [!CAUTION]
> **Resolute is a research project.**
>
> This software is experimental, unstable, and under active development. APIs will change without notice. Features may be incomplete or broken. There is no support, no documentation guarantees, and no warranty of any kind. Use at your own risk.

## Resources
- Website: https://resolute.sh
- Documentation: https://resolute.sh/docs
- Tutorials: https://resolute.sh/docs/getting-started

<p align="center">
  <img src="_docs/imgs/logo.svg" alt="Resolute" width="400">
</p>

Resolute is a **workflow-as-code framework** for Go, built on [Temporal](https://temporal.io). It provides type-safe abstractions that make building durable, fault-tolerant workflows feel natural.

## Features

- **Type-Safe Flows** - Generic nodes with compile-time type checking
- **Fluent Builder API** - Intuitive flow construction with `.Then()`, `.ThenParallel()`, `.When()`
- **Built-in Patterns** - Saga compensation, pagination, rate limiting out of the box
- **Provider Ecosystem** - Pre-built integrations for Jira, Confluence, Qdrant, Ollama, PagerDuty
- **Easy Testing** - `FlowTester` for unit tests without Temporal infrastructure
- **Production Ready** - Webhook triggers, state persistence, cursor-based incremental sync

## Getting Started & Documentation

Documentation is available on the [Resolute website](https://resolute.sh/docs):

- [Getting Started](https://resolute.sh/docs/getting-started) walks through installation and building your first workflow
- [Concepts](https://resolute.sh/docs/concepts) explains the core abstractions
- [Guides](https://resolute.sh/docs/guides) provides step-by-step tutorials for common patterns
- [API Reference](https://resolute.sh/docs/reference) covers the complete API

## Quick Example

```go
package main

import (
    "github.com/resolute-sh/resolute/core"
    "github.com/resolute-sh/resolute-jira"
    "github.com/resolute-sh/resolute-transform"
    "github.com/resolute-sh/resolute-qdrant"
)

func main() {
    // Define a workflow that syncs Jira issues to a vector database
    flow := core.NewFlow("knowledge-sync").
        TriggeredBy(core.Schedule("0 * * * *")).
        Then(jira.FetchIssues(jira.Input{
            Project: "PLATFORM",
            Since:   core.CursorFor("jira"),
        })).
        Then(transform.ChunkDocuments(transform.Input{
            MaxTokens: 512,
        })).
        Then(qdrant.Upsert(qdrant.Input{
            Collection: "knowledge-base",
        })).
        Build()

    // Run the worker
    core.NewWorker().
        WithConfig(core.WorkerConfig{TaskQueue: "knowledge-sync"}).
        WithFlow(flow).
        WithProviders(jira.Provider(), transform.Provider(), qdrant.Provider()).
        Run()
}
```

## Providers

| Provider | Package | Description |
|----------|---------|-------------|
| Jira | `resolute-jira` | Issue tracking integration |
| Confluence | `resolute-confluence` | Document management |
| Qdrant | `resolute-qdrant` | Vector database operations |
| Ollama | `resolute-ollama` | Local LLM embeddings |
| PagerDuty | `resolute-pagerduty` | Incident management |
| Transform | `resolute-transform` | Document chunking and merging |

## Contributing

We welcome contributions! See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

## License

MIT License - see [LICENSE](LICENSE) for details.
