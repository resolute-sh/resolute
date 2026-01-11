package core

import (
	"context"
	"fmt"
	"reflect"
	"sync"
)

// FlowTester provides a test harness for running flows without Temporal.
// It allows mocking activity implementations and asserting on execution.
type FlowTester struct {
	mu              sync.RWMutex
	mocks           map[string]mockEntry
	callCounts      map[string]int
	callArgs        map[string][]any
	applyRateLimit  bool
}

type mockEntry struct {
	fn         reflect.Value
	inputType  reflect.Type
	fixedValue any    // For MockValue - just return this value
	fixedError error  // For MockError - just return this error
}

// NewFlowTester creates a new test harness.
func NewFlowTester() *FlowTester {
	return &FlowTester{
		mocks:      make(map[string]mockEntry),
		callCounts: make(map[string]int),
		callArgs:   make(map[string][]any),
	}
}

// Mock registers a mock implementation for a node by name.
// The mock function should have signature: func(I) (O, error)
// where I and O match the node's input/output types.
//
// Example:
//
//	tester.Mock("jira.FetchIssues", func(input jira.FetchIssuesInput) (jira.FetchIssuesOutput, error) {
//	    return jira.FetchIssuesOutput{Count: 5}, nil
//	})
func (t *FlowTester) Mock(nodeName string, fn any) *FlowTester {
	t.mu.Lock()
	defer t.mu.Unlock()

	fnVal := reflect.ValueOf(fn)
	fnType := fnVal.Type()

	if fnType.Kind() != reflect.Func {
		panic(fmt.Sprintf("mock for %q must be a function, got %T", nodeName, fn))
	}
	if fnType.NumIn() != 1 {
		panic(fmt.Sprintf("mock for %q must have exactly 1 input parameter, got %d", nodeName, fnType.NumIn()))
	}
	if fnType.NumOut() != 2 {
		panic(fmt.Sprintf("mock for %q must have exactly 2 return values (output, error), got %d", nodeName, fnType.NumOut()))
	}
	if !fnType.Out(1).Implements(reflect.TypeOf((*error)(nil)).Elem()) {
		panic(fmt.Sprintf("mock for %q second return value must be error", nodeName))
	}

	t.mocks[nodeName] = mockEntry{
		fn:        fnVal,
		inputType: fnType.In(0),
	}

	return t
}

// MockValue registers a mock that always returns a fixed value.
//
// Example:
//
//	tester.MockValue("jira.FetchIssues", jira.FetchIssuesOutput{Count: 5})
func (t *FlowTester) MockValue(nodeName string, value any) *FlowTester {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.mocks[nodeName] = mockEntry{
		fixedValue: value,
	}
	return t
}

// MockError registers a mock that always returns an error.
//
// Example:
//
//	tester.MockError("jira.FetchIssues", errors.New("api error"))
func (t *FlowTester) MockError(nodeName string, err error) *FlowTester {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.mocks[nodeName] = mockEntry{
		fixedError: err,
	}
	return t
}

// Run executes the flow synchronously with mocked activities.
// Returns the final FlowState and any error encountered.
func (t *FlowTester) Run(flow *Flow, input FlowInput) (*FlowState, error) {
	return t.RunWithContext(context.Background(), flow, input)
}

// RunWithContext executes the flow with a context for cancellation support.
func (t *FlowTester) RunWithContext(ctx context.Context, flow *Flow, input FlowInput) (*FlowState, error) {
	state := NewFlowState(input)

	for _, step := range flow.Steps() {
		select {
		case <-ctx.Done():
			return state, ctx.Err()
		default:
		}

		if step.conditional != nil {
			if err := t.executeConditionalStep(ctx, step.conditional, state); err != nil {
				return state, err
			}
		} else if step.parallel {
			if err := t.executeParallelStep(ctx, step, state); err != nil {
				return state, err
			}
		} else {
			if err := t.executeSequentialStep(ctx, step, state); err != nil {
				return state, err
			}
		}
	}

	return state, nil
}

func (t *FlowTester) executeSequentialStep(ctx context.Context, step Step, state *FlowState) error {
	for _, node := range step.nodes {
		if err := t.executeNode(ctx, node, state); err != nil {
			return err
		}
	}
	return nil
}

func (t *FlowTester) executeParallelStep(ctx context.Context, step Step, state *FlowState) error {
	// For testing, we execute "parallel" steps sequentially
	// This is simpler and more deterministic for tests
	for _, node := range step.nodes {
		if err := t.executeNode(ctx, node, state); err != nil {
			return err
		}
	}
	return nil
}

func (t *FlowTester) executeConditionalStep(ctx context.Context, config *ConditionalConfig, state *FlowState) error {
	var stepsToExecute []Step
	if config.predicate(state) {
		stepsToExecute = config.thenSteps
	} else {
		stepsToExecute = config.elseSteps
	}

	for _, step := range stepsToExecute {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if step.conditional != nil {
			if err := t.executeConditionalStep(ctx, step.conditional, state); err != nil {
				return err
			}
		} else if step.parallel {
			if err := t.executeParallelStep(ctx, step, state); err != nil {
				return err
			}
		} else {
			if err := t.executeSequentialStep(ctx, step, state); err != nil {
				return err
			}
		}
	}

	return nil
}

func (t *FlowTester) executeNode(ctx context.Context, node ExecutableNode, state *FlowState) error {
	nodeName := node.Name()

	// Apply rate limiting if enabled and node has a rate limiter
	t.mu.RLock()
	applyRL := t.applyRateLimit
	t.mu.RUnlock()

	if applyRL {
		limiterID := node.RateLimiterID()
		if limiterID != "" {
			limiter := getRateLimiter(limiterID)
			if limiter != nil {
				if err := limiter.Wait(ctx); err != nil {
					return fmt.Errorf("rate limit %s: %w", nodeName, err)
				}
			}
		}
	}

	t.mu.Lock()
	mock, hasMock := t.mocks[nodeName]
	t.callCounts[nodeName]++
	t.mu.Unlock()

	if !hasMock {
		return fmt.Errorf("no mock registered for node %q", nodeName)
	}

	// Get the node's input
	nodeInput := t.getNodeInput(node)

	t.mu.Lock()
	t.callArgs[nodeName] = append(t.callArgs[nodeName], nodeInput)
	t.mu.Unlock()

	// Handle error mock
	if mock.fixedError != nil {
		return fmt.Errorf("execute %s: %w", nodeName, mock.fixedError)
	}

	// Handle fixed value mock
	if mock.fixedValue != nil {
		state.SetResult(node.OutputKey(), mock.fixedValue)
		return nil
	}

	// Resolve input markers before calling mock function
	resolvedInput, err := resolve(nodeInput, state)
	if err != nil {
		return fmt.Errorf("resolve input for %s: %w", nodeName, err)
	}

	// Call the mock function
	result, err := t.callMock(mock, resolvedInput)
	if err != nil {
		return fmt.Errorf("execute %s: %w", nodeName, err)
	}

	// Store result in state
	state.SetResult(node.OutputKey(), result)

	return nil
}

func (t *FlowTester) getNodeInput(node ExecutableNode) any {
	return node.Input()
}

func (t *FlowTester) callMock(mock mockEntry, input any) (any, error) {
	// Check if this is a function mock
	if !mock.fn.IsValid() {
		return nil, fmt.Errorf("invalid mock configuration: no function registered")
	}

	inputVal := reflect.ValueOf(input)

	// Convert input to expected type if needed
	if mock.inputType != nil && inputVal.Type() != mock.inputType {
		if inputVal.Type().ConvertibleTo(mock.inputType) {
			inputVal = inputVal.Convert(mock.inputType)
		}
	}

	results := mock.fn.Call([]reflect.Value{inputVal})

	resultVal := results[0].Interface()
	errVal := results[1].Interface()

	if errVal != nil {
		return nil, errVal.(error)
	}
	return resultVal, nil
}

// CallCount returns how many times a node was called.
func (t *FlowTester) CallCount(nodeName string) int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.callCounts[nodeName]
}

// WasCalled returns true if the node was called at least once.
func (t *FlowTester) WasCalled(nodeName string) bool {
	return t.CallCount(nodeName) > 0
}

// CallArgs returns all arguments passed to a node across all calls.
func (t *FlowTester) CallArgs(nodeName string) []any {
	t.mu.RLock()
	defer t.mu.RUnlock()
	args := make([]any, len(t.callArgs[nodeName]))
	copy(args, t.callArgs[nodeName])
	return args
}

// LastCallArg returns the argument from the last call to a node.
// Returns nil if the node was never called.
func (t *FlowTester) LastCallArg(nodeName string) any {
	args := t.CallArgs(nodeName)
	if len(args) == 0 {
		return nil
	}
	return args[len(args)-1]
}

// Reset clears all call tracking (but keeps mocks).
func (t *FlowTester) Reset() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.callCounts = make(map[string]int)
	t.callArgs = make(map[string][]any)
}

// WithRateLimiting enables rate limiting during test execution.
// By default, rate limiting is disabled in tests for faster execution.
// Enable this when you want to test rate limiting behavior.
func (t *FlowTester) WithRateLimiting() *FlowTester {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.applyRateLimit = true
	return t
}

// ResetAll clears all mocks and call tracking.
func (t *FlowTester) ResetAll() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.mocks = make(map[string]mockEntry)
	t.callCounts = make(map[string]int)
	t.callArgs = make(map[string][]any)
}

// TestingT is a subset of testing.T used for assertions.
type TestingT interface {
	Helper()
	Errorf(format string, args ...any)
}

// AssertCalled asserts that a node was called at least once.
func (t *FlowTester) AssertCalled(tb TestingT, nodeName string) {
	tb.Helper()
	if !t.WasCalled(nodeName) {
		tb.Errorf("expected %q to be called, but it wasn't", nodeName)
	}
}

// AssertNotCalled asserts that a node was not called.
func (t *FlowTester) AssertNotCalled(tb TestingT, nodeName string) {
	tb.Helper()
	if t.WasCalled(nodeName) {
		tb.Errorf("expected %q not to be called, but it was called %d times", nodeName, t.CallCount(nodeName))
	}
}

// AssertCallCount asserts that a node was called exactly n times.
func (t *FlowTester) AssertCallCount(tb TestingT, nodeName string, expected int) {
	tb.Helper()
	actual := t.CallCount(nodeName)
	if actual != expected {
		tb.Errorf("expected %q to be called %d times, got %d", nodeName, expected, actual)
	}
}
