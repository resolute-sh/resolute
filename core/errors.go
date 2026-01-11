package core

import (
	"errors"
	"fmt"
)

// ErrorType classifies errors for retry decisions.
type ErrorType int

const (
	// ErrorTypeRetryable indicates the error is transient (network timeouts, 5xx).
	ErrorTypeRetryable ErrorType = iota
	// ErrorTypeTerminal indicates a permanent failure (4xx auth/validation errors).
	ErrorTypeTerminal
	// ErrorTypeFatal indicates an unrecoverable error that should stop the workflow.
	ErrorTypeFatal
)

func (t ErrorType) String() string {
	switch t {
	case ErrorTypeRetryable:
		return "retryable"
	case ErrorTypeTerminal:
		return "terminal"
	case ErrorTypeFatal:
		return "fatal"
	default:
		return "unknown"
	}
}

// ErrorClassifier determines how an error should be handled for retries.
type ErrorClassifier func(error) ErrorType

// HTTPStatusError is implemented by errors that have an HTTP status code.
type HTTPStatusError interface {
	StatusCode() int
}

// HTTPErrorClassifier classifies errors based on HTTP status codes.
// 5xx and 429 are retryable, 4xx are terminal, others default to retryable.
func HTTPErrorClassifier(err error) ErrorType {
	var httpErr HTTPStatusError
	if errors.As(err, &httpErr) {
		code := httpErr.StatusCode()
		switch {
		case code >= 500:
			return ErrorTypeRetryable
		case code == 429:
			return ErrorTypeRetryable
		case code >= 400:
			return ErrorTypeTerminal
		}
	}
	return ErrorTypeRetryable
}

// ClassifiedError wraps an error with its classification.
type ClassifiedError struct {
	Err  error
	Type ErrorType
}

func (e *ClassifiedError) Error() string {
	return fmt.Sprintf("%s (%s)", e.Err.Error(), e.Type)
}

func (e *ClassifiedError) Unwrap() error {
	return e.Err
}

// IsRetryable returns true if the error should be retried.
func (e *ClassifiedError) IsRetryable() bool {
	return e.Type == ErrorTypeRetryable
}

// FlowError provides rich context for errors that occur during flow execution.
type FlowError struct {
	FlowName string
	StepName string
	NodeName string
	Input    interface{}
	Cause    error
}

func (e *FlowError) Error() string {
	return fmt.Sprintf("flow=%s step=%s node=%s: %v", e.FlowName, e.StepName, e.NodeName, e.Cause)
}

func (e *FlowError) Unwrap() error {
	return e.Cause
}

// WrapFlowError wraps an error with flow execution context.
// Returns nil if err is nil.
func WrapFlowError(flowName, stepName, nodeName string, input interface{}, err error) error {
	if err == nil {
		return nil
	}
	return &FlowError{
		FlowName: flowName,
		StepName: stepName,
		NodeName: nodeName,
		Input:    input,
		Cause:    err,
	}
}

// NodeError provides context for errors that occur during node execution.
type NodeError struct {
	NodeName string
	Input    interface{}
	Cause    error
}

func (e *NodeError) Error() string {
	return fmt.Sprintf("node=%s: %v", e.NodeName, e.Cause)
}

func (e *NodeError) Unwrap() error {
	return e.Cause
}

// WrapNodeError wraps an error with node execution context.
// Returns nil if err is nil.
func WrapNodeError(nodeName string, input interface{}, err error) error {
	if err == nil {
		return nil
	}
	return &NodeError{
		NodeName: nodeName,
		Input:    input,
		Cause:    err,
	}
}

// IsTerminalError checks if an error has been classified as terminal.
func IsTerminalError(err error) bool {
	var classified *ClassifiedError
	if errors.As(err, &classified) {
		return classified.Type == ErrorTypeTerminal
	}
	return false
}

// IsFatalError checks if an error has been classified as fatal.
func IsFatalError(err error) bool {
	var classified *ClassifiedError
	if errors.As(err, &classified) {
		return classified.Type == ErrorTypeFatal
	}
	return false
}
