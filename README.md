# Resolute

<p align="center">
  <b>Workflow-as-code framework for Go, built on Temporal</b>
</p>

<p align="center">
  <a href="https://github.com/resolute-sh/resolute/actions"><img src="https://github.com/resolute-sh/resolute/workflows/Go/badge.svg" alt="CI Status"></a>
  <a href="https://codecov.io/gh/resolute-sh/resolute"><img src="https://codecov.io/gh/resolute-sh/resolute/branch/main/graph/badge.svg" alt="Coverage"></a>
  <a href="https://pkg.go.dev/github.com/resolute-sh/resolute"><img src="https://pkg.go.dev/badge/github.com/resolute-sh/resolute.svg" alt="Go Reference"></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-MIT-blue.svg" alt="License"></a>
  <img src="https://img.shields.io/badge/go-1.22+-00ADD8.svg" alt="Go Version">
</p>

---

## What is Resolute?

Resolute is a developer-friendly framework for building durable, fault-tolerant workflows in Go. Built on [Temporal](https://temporal.io), it provides type-safe abstractions that make workflow development feel natural.

**Why Resolute?**

- Write workflows as Go code, not YAML or JSON
- Type-safe activities with compile-time checking
- Built-in patterns for common scenarios (pagination, rate limiting, compensation)
- Test workflows without spinning up infrastructure

## Features

- **Type-Safe Flows** - Generic nodes with compile-time type checking
- **Fluent Builder API** - Intuitive flow construction with `.Then()`, `.ThenParallel()`, `.When()`
- **Built-in Patterns** - Saga compensation, pagination, rate limiting out of the box
- **Provider Ecosystem** - Pre-built integrations for Jira, Confluence, Qdrant, Ollama, PagerDuty
- **Easy Testing** - `FlowTester` for unit tests without Temporal infrastructure
- **Production Ready** - Webhook triggers, state persistence, cursor-based incremental sync

## Quick Start

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
        TriggeredBy(core.Schedule("0 * * * *")).  // Every hour
        Then(jira.FetchIssues(jira.Input{
            Project: "PLATFORM",
            Since:   core.CursorFor("jira"),  // Incremental sync
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

## Installation

```bash
go get github.com/resolute-sh/resolute
```

## Core Concepts

### Flows

A **Flow** is a durable workflow composed of sequential and parallel steps:

```go
flow := core.NewFlow("my-flow").
    TriggeredBy(core.Schedule("0 * * * *")).
    Then(step1).
    ThenParallel("parallel-step", step2a, step2b).
    When(func(s *core.FlowState) bool {
        return core.Get[int](s, "count") > 100
    }).Then(step3).Else().Then(step4).
    Build()
```

### Nodes

A **Node** wraps a Temporal activity with typed input/output:

```go
node := core.NewNode("fetch-data", fetchDataActivity, input).
    WithRetry(core.RetryPolicy{MaxAttempts: 3}).
    WithTimeout(5 * time.Minute).
    WithRateLimit(100, time.Minute).
    As("fetched_data")  // Name the output for downstream reference
```

### Triggers

Flows can be triggered by:

- **Schedule**: Cron-based periodic execution
- **Manual**: API-initiated via trigger ID
- **Signal**: Temporal signal-based activation
- **Webhook**: HTTP webhook with signature verification

### State & Cursors

**FlowState** accumulates results from each step:

```go
// Access previous step outputs
data := core.Get[MyType](state, "step_name")

// Cursor-based incremental sync
Since: core.CursorFor("source_name")  // Auto-persisted, resumes from last position
```

## Documentation

| Resource | Description |
|----------|-------------|
| [Getting Started](https://resolute.sh/docs/getting-started) | Installation and first workflow |
| [Concepts](https://resolute.sh/docs/concepts) | Core abstractions explained |
| [Guides](https://resolute.sh/docs/guides) | Step-by-step tutorials |
| [API Reference](https://resolute.sh/docs/reference) | Complete API documentation |
| [Examples](https://resolute.sh/docs/examples) | Real-world workflow examples |

## Providers

| Provider | Package | Description |
|----------|---------|-------------|
| Jira | `resolute-jira` | Issue tracking integration |
| Confluence | `resolute-confluence` | Document management |
| Qdrant | `resolute-qdrant` | Vector database operations |
| Ollama | `resolute-ollama` | Local LLM embeddings |
| PagerDuty | `resolute-pagerduty` | Incident management |
| Transform | `resolute-transform` | Document chunking and merging |

## Testing

Use `FlowTester` for fast unit tests without Temporal:

```go
func TestMyFlow(t *testing.T) {
    tester := core.NewFlowTester()

    // Mock activities
    tester.Mock("fetch-data", func(input FetchInput) (FetchOutput, error) {
        return FetchOutput{Count: 42}, nil
    })

    // Run the flow
    state, err := tester.Run(myFlow, core.FlowInput{})
    require.NoError(t, err)

    // Assert results
    result := core.Get[FetchOutput](state, "fetch-data")
    assert.Equal(t, 42, result.Count)
}
```

## Contributing

We welcome contributions! See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

## License

MIT License - see [LICENSE](LICENSE) for details.
