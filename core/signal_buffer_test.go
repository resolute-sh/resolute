package core

import (
	"testing"
)

func TestSignalBuffer_Take(t *testing.T) {
	t.Parallel()

	t.Run("empty buffer returns false", func(t *testing.T) {
		t.Parallel()
		sb := newSignalBuffer()

		val, ok := sb.take("steer")
		if ok {
			t.Error("Take() should return false for empty buffer")
		}
		if val != nil {
			t.Errorf("Take() should return nil, got %v", val)
		}
	})

	t.Run("take returns injected signal", func(t *testing.T) {
		t.Parallel()
		sb := newSignalBuffer()
		sb.inject("steer", "hello")

		val, ok := sb.take("steer")
		if !ok {
			t.Error("Take() should return true after injection")
		}
		if val != "hello" {
			t.Errorf("Take() = %v, want %q", val, "hello")
		}
	})

	t.Run("take removes signal", func(t *testing.T) {
		t.Parallel()
		sb := newSignalBuffer()
		sb.inject("steer", "hello")
		sb.take("steer")

		_, ok := sb.take("steer")
		if ok {
			t.Error("Take() should return false after buffer is empty")
		}
	})

	t.Run("fifo order", func(t *testing.T) {
		t.Parallel()
		sb := newSignalBuffer()
		sb.inject("steer", "first")
		sb.inject("steer", "second")

		val, _ := sb.take("steer")
		if val != "first" {
			t.Errorf("first Take() = %v, want %q", val, "first")
		}
		val, _ = sb.take("steer")
		if val != "second" {
			t.Errorf("second Take() = %v, want %q", val, "second")
		}
	})
}

func TestSignalBuffer_TakeAll(t *testing.T) {
	t.Parallel()

	t.Run("empty buffer returns false", func(t *testing.T) {
		t.Parallel()
		sb := newSignalBuffer()

		_, ok := sb.takeAll("steer")
		if ok {
			t.Error("TakeAll() should return false for empty buffer")
		}
	})

	t.Run("returns all signals in order", func(t *testing.T) {
		t.Parallel()
		sb := newSignalBuffer()
		sb.inject("steer", "a")
		sb.inject("steer", "b")
		sb.inject("steer", "c")

		vals, ok := sb.takeAll("steer")
		if !ok {
			t.Error("TakeAll() should return true")
		}
		if len(vals) != 3 {
			t.Errorf("TakeAll() returned %d values, want 3", len(vals))
		}
		expected := []interface{}{"a", "b", "c"}
		for i, v := range vals {
			if v != expected[i] {
				t.Errorf("TakeAll()[%d] = %v, want %v", i, v, expected[i])
			}
		}
	})

	t.Run("clears buffer", func(t *testing.T) {
		t.Parallel()
		sb := newSignalBuffer()
		sb.inject("steer", "x")
		sb.takeAll("steer")

		if sb.size("steer") != 0 {
			t.Error("TakeAll() should clear the buffer")
		}
	})
}

func TestSignalBuffer_Peek(t *testing.T) {
	t.Parallel()

	t.Run("empty buffer returns false", func(t *testing.T) {
		t.Parallel()
		sb := newSignalBuffer()

		_, ok := sb.peek("steer")
		if ok {
			t.Error("Peek() should return false for empty buffer")
		}
	})

	t.Run("returns oldest without removing", func(t *testing.T) {
		t.Parallel()
		sb := newSignalBuffer()
		sb.inject("steer", "first")

		val, ok := sb.peek("steer")
		if !ok {
			t.Error("Peek() should return true")
		}
		if val != "first" {
			t.Errorf("Peek() = %v, want %q", val, "first")
		}

		val, ok = sb.peek("steer")
		if !ok {
			t.Error("Peek() should still return true after first peek")
		}
		if val != "first" {
			t.Errorf("Peek() = %v, want %q", val, "first")
		}
	})
}

func TestSignalBuffer_Len(t *testing.T) {
	t.Parallel()

	t.Run("empty buffer has len 0", func(t *testing.T) {
		t.Parallel()
		sb := newSignalBuffer()
		if sb.size("steer") != 0 {
			t.Error("Len() should be 0 for empty buffer")
		}
	})

	t.Run("counts buffered signals", func(t *testing.T) {
		t.Parallel()
		sb := newSignalBuffer()
		sb.inject("steer", "x")
		if sb.size("steer") != 1 {
			t.Errorf("Len() = %d, want 1", sb.size("steer"))
		}
	})

	t.Run("decrements after take", func(t *testing.T) {
		t.Parallel()
		sb := newSignalBuffer()
		sb.inject("steer", "x")
		sb.take("steer")
		if sb.size("steer") != 0 {
			t.Errorf("Len() = %d, want 0", sb.size("steer"))
		}
	})
}

func TestSignalBuffer_PerSignalIsolation(t *testing.T) {
	t.Parallel()

	t.Run("signals are isolated by name", func(t *testing.T) {
		t.Parallel()
		sb := newSignalBuffer()
		sb.inject("steer", "steer-msg")
		sb.inject("cancel", struct{}{})

		steer, ok := sb.take("steer")
		if !ok {
			t.Error("Take(steer) should return true")
		}
		if steer != "steer-msg" {
			t.Errorf("Take(steer) = %v, want %q", steer, "steer-msg")
		}

		cancel, ok := sb.take("cancel")
		if !ok {
			t.Error("Take(cancel) should return true")
		}
		if cancel != struct{}{} {
			t.Errorf("Take(cancel) = %v, want struct{}{}", cancel)
		}
	})
}

func TestSignalBuffer_MaxSizeDropOldest(t *testing.T) {
	t.Parallel()

	t.Run("drops oldest when max size exceeded", func(t *testing.T) {
		t.Parallel()
		sb := newSignalBuffer(withMaxSize(2))
		sb.inject("steer", "a")
		sb.inject("steer", "b")
		sb.inject("steer", "c")

		if sb.size("steer") != 2 {
			t.Errorf("Len() = %d, want 2", sb.size("steer"))
		}

		val, _ := sb.take("steer")
		if val != "b" {
			t.Errorf("first Take() = %v, want %q", val, "b")
		}
	})
}
