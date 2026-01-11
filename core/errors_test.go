package core

import (
	"errors"
	"fmt"
	"testing"
)

type httpError struct {
	code int
	msg  string
}

func (e httpError) Error() string   { return e.msg }
func (e httpError) StatusCode() int { return e.code }

func TestErrorType_String(t *testing.T) {
	t.Parallel()

	tests := []struct {
		errType ErrorType
		want    string
	}{
		{ErrorTypeRetryable, "retryable"},
		{ErrorTypeTerminal, "terminal"},
		{ErrorTypeFatal, "fatal"},
		{ErrorType(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			t.Parallel()
			if got := tt.errType.String(); got != tt.want {
				t.Errorf("ErrorType.String() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHTTPErrorClassifier(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want ErrorType
	}{
		{
			name: "500 error is retryable",
			err:  httpError{code: 500, msg: "internal server error"},
			want: ErrorTypeRetryable,
		},
		{
			name: "502 error is retryable",
			err:  httpError{code: 502, msg: "bad gateway"},
			want: ErrorTypeRetryable,
		},
		{
			name: "429 rate limit is retryable",
			err:  httpError{code: 429, msg: "too many requests"},
			want: ErrorTypeRetryable,
		},
		{
			name: "401 auth error is terminal",
			err:  httpError{code: 401, msg: "unauthorized"},
			want: ErrorTypeTerminal,
		},
		{
			name: "403 forbidden is terminal",
			err:  httpError{code: 403, msg: "forbidden"},
			want: ErrorTypeTerminal,
		},
		{
			name: "404 not found is terminal",
			err:  httpError{code: 404, msg: "not found"},
			want: ErrorTypeTerminal,
		},
		{
			name: "400 bad request is terminal",
			err:  httpError{code: 400, msg: "bad request"},
			want: ErrorTypeTerminal,
		},
		{
			name: "non-HTTP error defaults to retryable",
			err:  errors.New("connection refused"),
			want: ErrorTypeRetryable,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := HTTPErrorClassifier(tt.err); got != tt.want {
				t.Errorf("HTTPErrorClassifier() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestClassifiedError(t *testing.T) {
	t.Parallel()

	t.Run("Error includes type", func(t *testing.T) {
		t.Parallel()
		ce := &ClassifiedError{
			Err:  errors.New("something failed"),
			Type: ErrorTypeTerminal,
		}
		if ce.Error() == "" {
			t.Error("Error() should not be empty")
		}
	})

	t.Run("Unwrap returns cause", func(t *testing.T) {
		t.Parallel()
		cause := errors.New("root cause")
		ce := &ClassifiedError{Err: cause, Type: ErrorTypeRetryable}
		if !errors.Is(ce, cause) {
			t.Error("Unwrap should return the cause")
		}
	})

	t.Run("IsRetryable", func(t *testing.T) {
		t.Parallel()
		retryable := &ClassifiedError{Err: errors.New(""), Type: ErrorTypeRetryable}
		terminal := &ClassifiedError{Err: errors.New(""), Type: ErrorTypeTerminal}

		if !retryable.IsRetryable() {
			t.Error("retryable error should return true for IsRetryable")
		}
		if terminal.IsRetryable() {
			t.Error("terminal error should return false for IsRetryable")
		}
	})
}

func TestFlowError(t *testing.T) {
	t.Parallel()

	t.Run("Error message contains context", func(t *testing.T) {
		t.Parallel()
		fe := &FlowError{
			FlowName: "sync-flow",
			StepName: "fetch-data",
			NodeName: "jira-fetch",
			Input:    map[string]string{"project": "TEST"},
			Cause:    errors.New("connection timeout"),
		}

		errStr := fe.Error()
		if errStr == "" {
			t.Error("Error() should not be empty")
		}

		checks := []string{"sync-flow", "fetch-data", "jira-fetch", "connection timeout"}
		for _, check := range checks {
			found := false
			for i := 0; i < len(errStr)-len(check)+1; i++ {
				if errStr[i:i+len(check)] == check {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("Error() should contain %q, got %q", check, errStr)
			}
		}
	})

	t.Run("Unwrap returns cause", func(t *testing.T) {
		t.Parallel()
		cause := errors.New("root cause")
		fe := &FlowError{Cause: cause}
		if !errors.Is(fe, cause) {
			t.Error("Unwrap should return the cause")
		}
	})
}

func TestWrapFlowError(t *testing.T) {
	t.Parallel()

	t.Run("returns nil for nil error", func(t *testing.T) {
		t.Parallel()
		wrapped := WrapFlowError("flow", "step", "node", nil, nil)
		if wrapped != nil {
			t.Error("WrapFlowError should return nil for nil error")
		}
	})

	t.Run("wraps non-nil error", func(t *testing.T) {
		t.Parallel()
		cause := errors.New("failed")
		wrapped := WrapFlowError("myflow", "mystep", "mynode", "input", cause)

		var fe *FlowError
		if !errors.As(wrapped, &fe) {
			t.Error("wrapped error should be a FlowError")
		}

		if fe.FlowName != "myflow" {
			t.Errorf("FlowName = %q, want %q", fe.FlowName, "myflow")
		}
		if fe.StepName != "mystep" {
			t.Errorf("StepName = %q, want %q", fe.StepName, "mystep")
		}
		if fe.NodeName != "mynode" {
			t.Errorf("NodeName = %q, want %q", fe.NodeName, "mynode")
		}
	})
}

func TestNodeError(t *testing.T) {
	t.Parallel()

	t.Run("Error message contains node name", func(t *testing.T) {
		t.Parallel()
		ne := &NodeError{
			NodeName: "process-data",
			Input:    "test-input",
			Cause:    errors.New("processing failed"),
		}

		errStr := ne.Error()
		expected := "node=process-data"
		found := false
		for i := 0; i < len(errStr)-len(expected)+1; i++ {
			if errStr[i:i+len(expected)] == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Error() should contain %q, got %q", expected, errStr)
		}
	})

	t.Run("Unwrap returns cause", func(t *testing.T) {
		t.Parallel()
		cause := errors.New("root")
		ne := &NodeError{Cause: cause}
		if !errors.Is(ne, cause) {
			t.Error("Unwrap should return the cause")
		}
	})
}

func TestWrapNodeError(t *testing.T) {
	t.Parallel()

	t.Run("returns nil for nil error", func(t *testing.T) {
		t.Parallel()
		wrapped := WrapNodeError("node", nil, nil)
		if wrapped != nil {
			t.Error("WrapNodeError should return nil for nil error")
		}
	})

	t.Run("wraps non-nil error", func(t *testing.T) {
		t.Parallel()
		cause := errors.New("failed")
		wrapped := WrapNodeError("mynode", "input", cause)

		var ne *NodeError
		if !errors.As(wrapped, &ne) {
			t.Error("wrapped error should be a NodeError")
		}

		if ne.NodeName != "mynode" {
			t.Errorf("NodeName = %q, want %q", ne.NodeName, "mynode")
		}
	})
}

func TestIsTerminalError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "terminal classified error",
			err:  &ClassifiedError{Err: errors.New(""), Type: ErrorTypeTerminal},
			want: true,
		},
		{
			name: "retryable classified error",
			err:  &ClassifiedError{Err: errors.New(""), Type: ErrorTypeRetryable},
			want: false,
		},
		{
			name: "wrapped terminal error",
			err:  fmt.Errorf("wrapped: %w", &ClassifiedError{Err: errors.New(""), Type: ErrorTypeTerminal}),
			want: true,
		},
		{
			name: "plain error",
			err:  errors.New("plain error"),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := IsTerminalError(tt.err); got != tt.want {
				t.Errorf("IsTerminalError() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsFatalError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "fatal classified error",
			err:  &ClassifiedError{Err: errors.New(""), Type: ErrorTypeFatal},
			want: true,
		},
		{
			name: "terminal classified error",
			err:  &ClassifiedError{Err: errors.New(""), Type: ErrorTypeTerminal},
			want: false,
		},
		{
			name: "plain error",
			err:  errors.New("plain error"),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := IsFatalError(tt.err); got != tt.want {
				t.Errorf("IsFatalError() = %v, want %v", got, tt.want)
			}
		})
	}
}
