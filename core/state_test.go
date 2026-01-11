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
