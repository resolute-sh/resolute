package core

import (
	"testing"
)

func TestGetSafe(t *testing.T) {
	t.Parallel()

	t.Run("returns value when key exists", func(t *testing.T) {
		t.Parallel()
		state := &FlowState{results: make(map[string]interface{})}
		state.results["count"] = 42

		val, err := GetSafe[int](state, "count")
		if err != nil {
			t.Errorf("GetSafe() error = %v", err)
		}
		if val != 42 {
			t.Errorf("GetSafe() = %v, want %v", val, 42)
		}
	})

	t.Run("returns error when key not found", func(t *testing.T) {
		t.Parallel()
		state := &FlowState{results: make(map[string]interface{})}

		_, err := GetSafe[int](state, "missing")
		if err == nil {
			t.Error("GetSafe() should return error for missing key")
		}
	})

	t.Run("returns error on type mismatch", func(t *testing.T) {
		t.Parallel()
		state := &FlowState{results: make(map[string]interface{})}
		state.results["value"] = "string"

		_, err := GetSafe[int](state, "value")
		if err == nil {
			t.Error("GetSafe() should return error on type mismatch")
		}
	})
}

func TestMustGet(t *testing.T) {
	t.Parallel()

	t.Run("returns value when key exists", func(t *testing.T) {
		t.Parallel()
		state := &FlowState{results: make(map[string]interface{})}
		state.results["name"] = "test"

		val := MustGet[string](state, "name")
		if val != "test" {
			t.Errorf("MustGet() = %v, want %v", val, "test")
		}
	})

	t.Run("panics when key not found", func(t *testing.T) {
		t.Parallel()
		state := &FlowState{results: make(map[string]interface{})}

		defer func() {
			if r := recover(); r == nil {
				t.Error("MustGet() should panic for missing key")
			}
		}()

		MustGet[string](state, "missing")
	})

	t.Run("panics on type mismatch", func(t *testing.T) {
		t.Parallel()
		state := &FlowState{results: make(map[string]interface{})}
		state.results["value"] = 123

		defer func() {
			if r := recover(); r == nil {
				t.Error("MustGet() should panic on type mismatch")
			}
		}()

		MustGet[string](state, "value")
	})
}

func TestHas(t *testing.T) {
	t.Parallel()

	t.Run("returns true for existing key", func(t *testing.T) {
		t.Parallel()
		state := &FlowState{results: make(map[string]interface{})}
		state.results["exists"] = "value"

		if !Has(state, "exists") {
			t.Error("Has() should return true for existing key")
		}
	})

	t.Run("returns false for missing key", func(t *testing.T) {
		t.Parallel()
		state := &FlowState{results: make(map[string]interface{})}

		if Has(state, "missing") {
			t.Error("Has() should return false for missing key")
		}
	})

	t.Run("returns true for nil value", func(t *testing.T) {
		t.Parallel()
		state := &FlowState{results: make(map[string]interface{})}
		state.results["nil-value"] = nil

		if !Has(state, "nil-value") {
			t.Error("Has() should return true even if value is nil")
		}
	})
}

func TestKeys(t *testing.T) {
	t.Parallel()

	t.Run("returns empty slice for empty state", func(t *testing.T) {
		t.Parallel()
		state := &FlowState{results: make(map[string]interface{})}

		keys := Keys(state)
		if len(keys) != 0 {
			t.Errorf("Keys() should return empty slice, got %v", keys)
		}
	})

	t.Run("returns sorted keys", func(t *testing.T) {
		t.Parallel()
		state := &FlowState{results: make(map[string]interface{})}
		state.results["zebra"] = 1
		state.results["alpha"] = 2
		state.results["beta"] = 3

		keys := Keys(state)
		if len(keys) != 3 {
			t.Errorf("Keys() should return 3 keys, got %d", len(keys))
		}

		expected := []string{"alpha", "beta", "zebra"}
		for i, key := range keys {
			if key != expected[i] {
				t.Errorf("Keys()[%d] = %q, want %q", i, key, expected[i])
			}
		}
	})
}

func TestGetOr(t *testing.T) {
	t.Parallel()

	t.Run("returns value when key exists", func(t *testing.T) {
		t.Parallel()
		state := &FlowState{results: make(map[string]interface{})}
		state.results["count"] = 100

		val := GetOr(state, "count", 0)
		if val != 100 {
			t.Errorf("GetOr() = %v, want %v", val, 100)
		}
	})

	t.Run("returns default when key not found", func(t *testing.T) {
		t.Parallel()
		state := &FlowState{results: make(map[string]interface{})}

		val := GetOr(state, "missing", 42)
		if val != 42 {
			t.Errorf("GetOr() = %v, want default %v", val, 42)
		}
	})

	t.Run("returns default on type mismatch", func(t *testing.T) {
		t.Parallel()
		state := &FlowState{results: make(map[string]interface{})}
		state.results["value"] = "string"

		val := GetOr(state, "value", 99)
		if val != 99 {
			t.Errorf("GetOr() = %v, want default %v", val, 99)
		}
	})
}

func TestGet(t *testing.T) {
	t.Parallel()

	t.Run("returns value when key exists", func(t *testing.T) {
		t.Parallel()
		state := &FlowState{results: make(map[string]interface{})}
		state.results["data"] = []string{"a", "b"}

		val := Get[[]string](state, "data")
		if len(val) != 2 {
			t.Errorf("Get() = %v, want slice with 2 elements", val)
		}
	})

	t.Run("panics when key not found", func(t *testing.T) {
		t.Parallel()
		state := &FlowState{results: make(map[string]interface{})}

		defer func() {
			if r := recover(); r == nil {
				t.Error("Get() should panic for missing key")
			}
		}()

		Get[string](state, "missing")
	})
}

func TestSet(t *testing.T) {
	t.Parallel()

	t.Run("stores value", func(t *testing.T) {
		t.Parallel()
		state := &FlowState{results: make(map[string]interface{})}

		Set(state, "key", "value")

		if !Has(state, "key") {
			t.Error("Set() should store the value")
		}

		val := Get[string](state, "key")
		if val != "value" {
			t.Errorf("Set() stored %q, want %q", val, "value")
		}
	})

	t.Run("overwrites existing value", func(t *testing.T) {
		t.Parallel()
		state := &FlowState{results: make(map[string]interface{})}
		state.results["key"] = "old"

		Set(state, "key", "new")

		val := Get[string](state, "key")
		if val != "new" {
			t.Errorf("Set() should overwrite, got %q, want %q", val, "new")
		}
	})
}

func TestFlowState_ChildWorkflows(t *testing.T) {
	t.Parallel()

	t.Run("empty state has no children", func(t *testing.T) {
		t.Parallel()
		state := NewFlowState(FlowInput{})

		_, ok := state.ChildWorkflowID("agent")
		if ok {
			t.Error("ChildWorkflowID should return false for empty state")
		}

		all := state.ChildWorkflows()
		if len(all) != 0 {
			t.Errorf("ChildWorkflows() = %v, want empty", all)
		}
	})

	t.Run("register and retrieve child workflow", func(t *testing.T) {
		t.Parallel()
		state := NewFlowState(FlowInput{})

		state.RegisterChildWorkflow("agent", "child-123")
		id, ok := state.ChildWorkflowID("agent")
		if !ok {
			t.Error("ChildWorkflowID should return true after registration")
		}
		if id != "child-123" {
			t.Errorf("ChildWorkflowID = %q, want %q", id, "child-123")
		}

		all := state.ChildWorkflows()
		if len(all) != 1 {
			t.Errorf("ChildWorkflows() len = %d, want 1", len(all))
		}
		if all["agent"] != "child-123" {
			t.Errorf("ChildWorkflows()[agent] = %q, want %q", all["agent"], "child-123")
		}
	})

	t.Run("snapshot copies child workflows", func(t *testing.T) {
		t.Parallel()
		state := NewFlowState(FlowInput{})
		state.RegisterChildWorkflow("agent", "child-123")

		snapshot := state.Snapshot()
		id, ok := snapshot.ChildWorkflowID("agent")
		if !ok || id != "child-123" {
			t.Error("Snapshot should copy childWorkflows")
		}
	})

	t.Run("batch state has empty child workflows", func(t *testing.T) {
		t.Parallel()
		state := NewFlowState(FlowInput{})
		state.RegisterChildWorkflow("agent", "child-123")

		batch := state.NewBatchState()
		_, ok := batch.ChildWorkflowID("agent")
		if ok {
			t.Error("NewBatchState should have empty childWorkflows")
		}
	})
}

func TestFlowState_Signals(t *testing.T) {
	t.Parallel()

	t.Run("new state has empty signal buffer", func(t *testing.T) {
		t.Parallel()
		state := NewFlowState(FlowInput{})

		if state.Signals() == nil {
			t.Error("NewFlowState() should initialize Signals")
		}
		if state.Signals().Len("steer") != 0 {
			t.Error("Signals should start empty")
		}
	})

	t.Run("signals can be injected and taken", func(t *testing.T) {
		t.Parallel()
		state := NewFlowState(FlowInput{})

		state.Signals().Inject("steer", "msg1")
		if state.Signals().Len("steer") != 1 {
			t.Errorf("Len() = %d, want 1", state.Signals().Len("steer"))
		}

		val, ok := state.Signals().Take("steer")
		if !ok {
			t.Error("Take() should return true")
		}
		if val != "msg1" {
			t.Errorf("Take() = %v, want %q", val, "msg1")
		}
	})

	t.Run("snapshot has fresh empty buffer", func(t *testing.T) {
		t.Parallel()
		state := NewFlowState(FlowInput{})
		state.Signals().Inject("steer", "x")

		snapshot := state.Snapshot()
		if snapshot.Signals().Len("steer") != 0 {
			t.Error("Snapshot should have empty signal buffer")
		}
	})

	t.Run("batch state has fresh empty buffer", func(t *testing.T) {
		t.Parallel()
		state := NewFlowState(FlowInput{})
		state.Signals().Inject("steer", "x")

		batch := state.NewBatchState()
		if batch.Signals().Len("steer") != 0 {
			t.Error("NewBatchState should have empty signal buffer")
		}
	})
}

func TestFlowStateReader_FlowStateSatisfiesInterface(t *testing.T) {
	// Compile-time check: *FlowState must satisfy FlowStateReader.
	var _ FlowStateReader = (*FlowState)(nil)

	// Runtime check: a populated FlowState read through the interface
	// returns the same values as direct *FlowState access.
	fs := NewFlowState(FlowInput{})
	Set(fs, "answer", 42)

	var reader FlowStateReader = fs
	got := Get[int](reader, "answer")
	if got != 42 {
		t.Fatalf("Get via FlowStateReader = %d, want 42", got)
	}
	if !Has(reader, "answer") {
		t.Fatal("Has via FlowStateReader returned false for present key")
	}
	keys := Keys(reader)
	if len(keys) != 1 || keys[0] != "answer" {
		t.Fatalf("Keys via FlowStateReader = %v, want [answer]", keys)
	}
}
