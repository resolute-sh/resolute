package core

import (
	"context"
	"testing"
	"time"
)

// Test activity for unit tests
func testActivity(ctx context.Context, input testInput) (testOutput, error) {
	return testOutput{Result: input.Value + "_processed"}, nil
}

type testInput struct {
	Value string
}

type testOutput struct {
	Result string
}

func TestNewNode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		nodeName string
		input    testInput
		wantName string
	}{
		{
			name:     "creates node with name",
			nodeName: "test-node",
			input:    testInput{Value: "test"},
			wantName: "test-node",
		},
		{
			name:     "creates node with empty input",
			nodeName: "empty-input-node",
			input:    testInput{},
			wantName: "empty-input-node",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			node := NewNode(tt.nodeName, testActivity, tt.input)

			if node.Name() != tt.wantName {
				t.Errorf("Name() = %q, want %q", node.Name(), tt.wantName)
			}
		})
	}
}

func TestNode_WithRetry(t *testing.T) {
	t.Parallel()

	node := NewNode("retry-test", testActivity, testInput{}).
		WithRetry(RetryPolicy{
			InitialInterval:    2 * time.Second,
			BackoffCoefficient: 3.0,
			MaximumInterval:    30 * time.Second,
			MaximumAttempts:    5,
		})

	if node.options.RetryPolicy == nil {
		t.Fatal("RetryPolicy is nil")
	}
	if node.options.RetryPolicy.MaximumAttempts != 5 {
		t.Errorf("MaximumAttempts = %d, want 5", node.options.RetryPolicy.MaximumAttempts)
	}
	if node.options.RetryPolicy.InitialInterval != 2*time.Second {
		t.Errorf("InitialInterval = %v, want 2s", node.options.RetryPolicy.InitialInterval)
	}
}

func TestNode_WithTimeout(t *testing.T) {
	t.Parallel()

	node := NewNode("timeout-test", testActivity, testInput{}).
		WithTimeout(10 * time.Minute)

	if node.options.StartToCloseTimeout != 10*time.Minute {
		t.Errorf("StartToCloseTimeout = %v, want 10m", node.options.StartToCloseTimeout)
	}
}

func TestNode_As(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		nodeName  string
		outputKey string
		wantKey   string
	}{
		{
			name:      "custom output key",
			nodeName:  "my-node",
			outputKey: "custom-key",
			wantKey:   "custom-key",
		},
		{
			name:      "defaults to node name when empty",
			nodeName:  "default-node",
			outputKey: "",
			wantKey:   "default-node",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			node := NewNode(tt.nodeName, testActivity, testInput{})
			if tt.outputKey != "" {
				node = node.As(tt.outputKey)
			}

			if node.OutputKey() != tt.wantKey {
				t.Errorf("OutputKey() = %q, want %q", node.OutputKey(), tt.wantKey)
			}
		})
	}
}

func TestNode_OnError(t *testing.T) {
	t.Parallel()

	compensationNode := NewNode("compensation", testActivity, testInput{})
	node := NewNode("main-node", testActivity, testInput{}).
		OnError(compensationNode)

	if !node.HasCompensation() {
		t.Error("HasCompensation() = false, want true")
	}
	if node.Compensation() != compensationNode {
		t.Error("Compensation() returned wrong node")
	}
}

func TestNode_NoCompensation(t *testing.T) {
	t.Parallel()

	node := NewNode("no-comp-node", testActivity, testInput{})

	if node.HasCompensation() {
		t.Error("HasCompensation() = true, want false")
	}
	if node.Compensation() != nil {
		t.Error("Compensation() should be nil")
	}
}

func TestDefaultActivityOptions(t *testing.T) {
	t.Parallel()

	opts := DefaultActivityOptions()

	if opts.StartToCloseTimeout != 5*time.Minute {
		t.Errorf("StartToCloseTimeout = %v, want 5m", opts.StartToCloseTimeout)
	}
	if opts.RetryPolicy == nil {
		t.Fatal("RetryPolicy is nil")
	}
	if opts.RetryPolicy.MaximumAttempts != 3 {
		t.Errorf("MaximumAttempts = %d, want 3", opts.RetryPolicy.MaximumAttempts)
	}
}
