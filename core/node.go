// Package core provides the fundamental primitives for building resolute workflows.
package core

import (
	"context"
	"fmt"
	"reflect"
	"time"

	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

// CursorUpdateConfig defines how a node updates a cursor after execution.
type CursorUpdateConfig struct {
	Source string
	Field  string
}

// Window configures batched/windowed processing for a node.
// When attached to a node in a parallel step, the framework loops:
// fetch batch → run downstream pipeline → persist cursors → repeat.
type Window struct {
	Size int // max items per batch (0 = disabled, fetch all)
}

// Node represents a single Temporal Activity with typed input/output.
// Provider functions return ready-to-use nodes with direct struct inputs.
type Node[I, O any] struct {
	name            string
	activity        func(context.Context, I) (O, error)
	input           I
	options         ActivityOptions
	outputKey       string
	compensation    ExecutableNode
	rateLimiterID   string
	validate        bool
	errorClassifier ErrorClassifier
	cursorUpdate    *CursorUpdateConfig
	window          *Window
}

// ActivityOptions configures retry and timeout behavior for a node.
type ActivityOptions struct {
	RetryPolicy        *RetryPolicy
	StartToCloseTimeout time.Duration
	HeartbeatTimeout    time.Duration
	TaskQueue           string
}

// RetryPolicy defines retry behavior for failed activities.
type RetryPolicy struct {
	InitialInterval    time.Duration
	BackoffCoefficient float64
	MaximumInterval    time.Duration
	MaximumAttempts    int32
}

// DefaultActivityOptions returns sensible defaults for activity execution.
func DefaultActivityOptions() ActivityOptions {
	return ActivityOptions{
		StartToCloseTimeout: 5 * time.Minute,
		RetryPolicy: &RetryPolicy{
			InitialInterval:    time.Second,
			BackoffCoefficient: 2.0,
			MaximumInterval:    time.Minute,
			MaximumAttempts:    3,
		},
	}
}

// NewNode creates a node wrapping an activity function.
// This is typically called by provider packages, not directly by users.
func NewNode[I, O any](name string, activity func(context.Context, I) (O, error), input I) *Node[I, O] {
	return &Node[I, O]{
		name:     name,
		activity: activity,
		input:    input,
		options:  DefaultActivityOptions(),
	}
}

// WithRetry configures the retry policy for this node.
func (n *Node[I, O]) WithRetry(policy RetryPolicy) *Node[I, O] {
	n.options.RetryPolicy = &policy
	return n
}

// WithTimeout sets the start-to-close timeout for this node.
func (n *Node[I, O]) WithTimeout(d time.Duration) *Node[I, O] {
	n.options.StartToCloseTimeout = d
	return n
}

// OnError attaches a compensation node to run if subsequent steps fail (Saga pattern).
func (n *Node[I, O]) OnError(compensation ExecutableNode) *Node[I, O] {
	n.compensation = compensation
	return n
}

// WithRateLimit configures rate limiting for this node.
// requests is the maximum number of requests allowed per duration.
// The rate limiter is unique to this node instance.
//
// Example:
//
//	node := jira.FetchIssues(config).WithRateLimit(100, time.Minute)
func (n *Node[I, O]) WithRateLimit(requests int, per time.Duration) *Node[I, O] {
	limiter := NewSharedRateLimiter(n.name, requests, per)
	n.rateLimiterID = limiter.ID()
	return n
}

// WithSharedRateLimit configures this node to use a shared rate limiter.
// Multiple nodes can share the same rate limiter to coordinate request rates.
//
// Example:
//
//	limiter := core.NewSharedRateLimiter("jira-api", 100, time.Minute)
//	node1 := jira.FetchIssues(config).WithSharedRateLimit(limiter)
//	node2 := jira.SearchJQL(config).WithSharedRateLimit(limiter)
func (n *Node[I, O]) WithSharedRateLimit(limiter *SharedRateLimiter) *Node[I, O] {
	n.rateLimiterID = limiter.ID()
	return n
}

// As names the output of this node for reference by downstream nodes.
func (n *Node[I, O]) As(outputKey string) *Node[I, O] {
	n.outputKey = outputKey
	return n
}

// WithValidation enables input validation using struct tags before execution.
// Validation tags: required, min=N, max=N, minlen=N, maxlen=N, oneof=a|b|c
//
// Example:
//
//	type Input struct {
//	    Name string `validate:"required"`
//	    Age  int    `validate:"min=0,max=150"`
//	}
func (n *Node[I, O]) WithValidation() *Node[I, O] {
	n.validate = true
	return n
}

// WithErrorClassifier sets a function to classify errors for retry decisions.
// Terminal errors are marked as non-retryable for Temporal.
//
// Example:
//
//	node.WithErrorClassifier(core.HTTPErrorClassifier)
func (n *Node[I, O]) WithErrorClassifier(fn ErrorClassifier) *Node[I, O] {
	n.errorClassifier = fn
	return n
}

// WithCursorUpdate configures the node to update a cursor after successful execution.
// The named field is extracted from the activity output and persisted as the cursor position.
func (n *Node[I, O]) WithCursorUpdate(source, field string) *Node[I, O] {
	n.cursorUpdate = &CursorUpdateConfig{Source: source, Field: field}
	return n
}

// WithWindow configures batched/windowed processing for this node.
// When used in a parallel step, the framework runs the downstream pipeline
// per batch instead of waiting for all data.
func (n *Node[I, O]) WithWindow(w Window) *Node[I, O] {
	n.window = &w
	return n
}

// WindowConfig returns the window configuration. Returns zero Window if not set.
func (n *Node[I, O]) WindowConfig() Window {
	if n.window == nil {
		return Window{}
	}
	return *n.window
}

// Name returns the node's identifier.
func (n *Node[I, O]) Name() string {
	return n.name
}

// OutputKey returns the key used to store this node's output.
func (n *Node[I, O]) OutputKey() string {
	if n.outputKey != "" {
		return n.outputKey
	}
	return n.name
}

// HasCompensation returns true if this node has a compensation handler.
func (n *Node[I, O]) HasCompensation() bool {
	return n.compensation != nil
}

// Compensation returns the compensation node, if any.
func (n *Node[I, O]) Compensation() ExecutableNode {
	return n.compensation
}

// Input returns the node's input value (used for testing).
func (n *Node[I, O]) Input() any {
	return n.input
}

// RateLimiterID returns the rate limiter ID for this node.
func (n *Node[I, O]) RateLimiterID() string {
	return n.rateLimiterID
}

// Execute runs the activity within a Temporal workflow context.
func (n *Node[I, O]) Execute(ctx workflow.Context, state *FlowState) error {
	startTime := time.Now()

	if n.rateLimiterID != "" {
		if err := executeWithRateLimit(ctx, n.rateLimiterID); err != nil {
			return fmt.Errorf("rate limit %s: %w", n.name, err)
		}
	}

	resolvedInput, err := resolveInput(ctx, n.input, state)
	if err != nil {
		RecordActivityExecution(RecordActivityExecutionInput{
			NodeName: n.name,
			Duration: time.Since(startTime),
			Err:      err,
		})
		return err
	}

	if n.window != nil {
		meta := state.GetWindowMeta()
		injectWindowFields(&resolvedInput, meta.Cursor, meta.Size)
	}

	if n.validate {
		if err := Validate(resolvedInput); err != nil {
			validationErr := fmt.Errorf("input validation failed for %s: %w", n.name, err)
			RecordActivityExecution(RecordActivityExecutionInput{
				NodeName: n.name,
				Duration: time.Since(startTime),
				Err:      validationErr,
			})
			return validationErr
		}
	}

	activityOpts := workflow.ActivityOptions{
		StartToCloseTimeout: n.options.StartToCloseTimeout,
		HeartbeatTimeout:    n.options.HeartbeatTimeout,
		TaskQueue:           n.options.TaskQueue,
	}

	if n.options.RetryPolicy != nil {
		activityOpts.RetryPolicy = &temporal.RetryPolicy{
			InitialInterval:    n.options.RetryPolicy.InitialInterval,
			BackoffCoefficient: n.options.RetryPolicy.BackoffCoefficient,
			MaximumInterval:    n.options.RetryPolicy.MaximumInterval,
			MaximumAttempts:    n.options.RetryPolicy.MaximumAttempts,
		}
	}

	ctx = workflow.WithActivityOptions(ctx, activityOpts)

	var result O
	err = workflow.ExecuteActivity(ctx, n.activity, resolvedInput).Get(ctx, &result)

	RecordActivityExecution(RecordActivityExecutionInput{
		NodeName: n.name,
		Duration: time.Since(startTime),
		Err:      err,
	})

	if err != nil {
		return n.classifyError(err)
	}

	state.SetResult(n.OutputKey(), result)

	if n.cursorUpdate != nil {
		extractAndSetCursor(state, result, n.cursorUpdate)
	}

	return nil
}

// classifyError wraps errors with classification for retry decisions.
func (n *Node[I, O]) classifyError(err error) error {
	if n.errorClassifier == nil {
		return err
	}

	errType := n.errorClassifier(err)
	if errType == ErrorTypeTerminal || errType == ErrorTypeFatal {
		return temporal.NewNonRetryableApplicationError(
			err.Error(),
			errType.String(),
			err,
		)
	}
	return err
}

// Compensate runs the compensation activity if one is configured.
func (n *Node[I, O]) Compensate(ctx workflow.Context, state *FlowState) error {
	if n.compensation == nil {
		return nil
	}
	return n.compensation.Execute(ctx, state)
}

// extractAndSetCursor extracts a time field from a result and updates the cursor in state.
func extractAndSetCursor[O any](state *FlowState, result O, cfg *CursorUpdateConfig) {
	val := reflect.ValueOf(result)
	if val.Kind() == reflect.Ptr {
		val = val.Elem()
	}
	if val.Kind() != reflect.Struct {
		return
	}

	field := val.FieldByName(cfg.Field)
	if !field.IsValid() {
		return
	}

	var position string
	switch v := field.Interface().(type) {
	case time.Time:
		if !v.IsZero() {
			position = v.Format(time.RFC3339)
		}
	case *time.Time:
		if v != nil && !v.IsZero() {
			position = v.Format(time.RFC3339)
		}
	case string:
		position = v
	}

	if position != "" {
		state.SetCursor(cfg.Source, position)
	}
}

// injectWindowFields sets WindowCursor and WindowSize fields on an input struct via reflection.
func injectWindowFields[I any](input *I, cursor string, size int) {
	val := reflect.ValueOf(input).Elem()
	if val.Kind() != reflect.Struct {
		return
	}
	if f := val.FieldByName("WindowCursor"); f.IsValid() && f.CanSet() && f.Kind() == reflect.String {
		f.SetString(cursor)
	}
	if f := val.FieldByName("WindowSize"); f.IsValid() && f.CanSet() && f.Kind() == reflect.Int {
		f.SetInt(int64(size))
	}
}

// resolveInput processes the input struct to replace magic markers with actual values.
func resolveInput[I any](ctx workflow.Context, input I, state *FlowState) (I, error) {
	// The actual resolution happens via reflection in the resolver package.
	// Magic markers (CursorRef, OutputRef) are detected and replaced.
	return resolve(input, state)
}
