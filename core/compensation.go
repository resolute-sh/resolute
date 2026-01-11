package core

// CompensationEntry records a node execution for potential rollback.
// Used by the Saga pattern to track which compensations need to run on failure.
type CompensationEntry struct {
	// node is the executed node that has compensation logic.
	node ExecutableNode

	// state is a snapshot of FlowState at the time of node execution.
	// This allows compensation to access the original context/outputs.
	state *FlowState
}

// CompensationChain manages the ordered list of compensation entries.
// Compensations are executed in reverse order (LIFO) on failure.
type CompensationChain struct {
	entries []CompensationEntry
}

// NewCompensationChain creates an empty compensation chain.
func NewCompensationChain() *CompensationChain {
	return &CompensationChain{
		entries: make([]CompensationEntry, 0),
	}
}

// Add records a new compensation entry.
func (c *CompensationChain) Add(node ExecutableNode, state *FlowState) {
	if node == nil || !node.HasCompensation() {
		return
	}
	c.entries = append(c.entries, CompensationEntry{
		node:  node,
		state: state.Snapshot(),
	})
}

// Entries returns all compensation entries in execution order.
func (c *CompensationChain) Entries() []CompensationEntry {
	return c.entries
}

// Reversed returns compensation entries in reverse order for rollback.
func (c *CompensationChain) Reversed() []CompensationEntry {
	n := len(c.entries)
	reversed := make([]CompensationEntry, n)
	for i, entry := range c.entries {
		reversed[n-1-i] = entry
	}
	return reversed
}

// Len returns the number of compensation entries.
func (c *CompensationChain) Len() int {
	return len(c.entries)
}

// Clear removes all compensation entries (called after successful completion).
func (c *CompensationChain) Clear() {
	c.entries = c.entries[:0]
}
