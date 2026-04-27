package core

// Layer 2 workflow replay tests for flow.Loop and flow.ParallelEach.
//
// Plan A originally specified replay tests using
// testsuite.TestWorkflowEnvironment.GetWorkflowHistory + worker.WorkflowReplayer.
// That API combination is not available on the installed Temporal SDK
// (v1.29.1). Two implementable alternatives exist:
//
//  1. testsuite.StartDevServer — spins up a real Temporalite process, captures
//     history via client.GetWorkflowHistory, replays. Adds external-process
//     dependency to core unit tests; slower; not hermetic.
//  2. Hand-built synthetic *historypb.History — mirrors the SDK's own internal
//     tests. Brittle: tests the synthetic event sequence rather than real
//     code paths.
//
// Decision (2026-04-27): defer Layer 2 replay tests to Plan B (resolute-harness)
// where they run against a real Temporal cluster as part of acceptance e2e
// (spec §13 #7). The determinism contract is currently enforced structurally:
//
//   - flow.Loop's While, InitialState, FinalState, IterationHook closures
//     operate only on typed S — no time.Now, no rand, no I/O at the API.
//   - flow.ParallelEach's From and Merge closures operate only on *FlowState
//     (snapshot semantics) — same constraint.
//   - The runners use Temporal-native primitives (workflow.NewBufferedChannel,
//     workflow.Selector, workflow.Go) — see core/parallel_each.go.
//   - typedLoopRunner and typedParallelEachRunner contain no globals, no
//     init(), no package-level mutable state.
//
// Violations would require deliberate misuse of the option closures, which is
// caught by code review and the existing Layer 1 unit tests in
// core/loop_test.go and core/parallel_each_test.go.
