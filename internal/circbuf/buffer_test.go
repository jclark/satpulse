package circbuf

import (
	"testing"
)

func TestBuffer(t *testing.T) {
	t.Run("NewPanicsOnInvalidSize", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Error("expected panic for size <= 0")
			}
		}()
		New[int](0)
	})

	t.Run("BasicOperations", func(t *testing.T) {
		b := New[int](3)

		if b.Cap() != 3 {
			t.Errorf("Cap: got %d, want 3", b.Cap())
		}

		if b.Len() != 0 {
			t.Errorf("Len on empty: got %d, want 0", b.Len())
		}

		// Add some elements
		b.Append(1)
		b.Append(2)

		if b.Len() != 2 {
			t.Errorf("Len after 2 appends: got %d, want 2", b.Len())
		}

		if b.Last(0) != 2 {
			t.Errorf("Last(0): got %d, want 2", b.Last(0))
		}

		if b.Last(1) != 1 {
			t.Errorf("Last(1): got %d, want 1", b.Last(1))
		}

		// Fill buffer completely
		b.Append(3)

		if b.Len() != 3 {
			t.Errorf("Len when full: got %d, want 3", b.Len())
		}

		// Overflow - should overwrite oldest
		b.Append(4)

		if b.Len() != 3 {
			t.Errorf("Len after overflow: got %d, want 3", b.Len())
		}

		if b.Last(0) != 4 {
			t.Errorf("Last(0) after overflow: got %d, want 4", b.Last(0))
		}

		if b.Last(1) != 3 {
			t.Errorf("Last(1) after overflow: got %d, want 3", b.Last(1))
		}

		if b.Last(2) != 2 {
			t.Errorf("Last(2) after overflow: got %d, want 2", b.Last(2))
		}
	})

	t.Run("Clear", func(t *testing.T) {
		b := New[int](3)
		b.Append(1)
		b.Append(2)
		b.Append(3)

		b.Clear()

		if b.Len() != 0 {
			t.Errorf("Len after Clear: got %d, want 0", b.Len())
		}

		if b.Last(0) != 0 {
			t.Errorf("Last(0) after Clear: got %d, want 0", b.Last(0))
		}
	})

	t.Run("LastOutOfRange", func(t *testing.T) {
		b := New[int](3)
		b.Append(1)
		b.Append(2)

		if b.Last(-1) != 0 {
			t.Errorf("Last(-1): got %d, want 0", b.Last(-1))
		}

		if b.Last(2) != 0 {
			t.Errorf("Last(2) when len=2: got %d, want 0", b.Last(2))
		}

		if b.Last(100) != 0 {
			t.Errorf("Last(100): got %d, want 0", b.Last(100))
		}
	})

	t.Run("Iterate", func(t *testing.T) {
		b := New[int](3)
		b.Append(1)
		b.Append(2)
		b.Append(3)

		var values []int
		var indices []int
		b.Iterate(func(i, v int) bool {
			indices = append(indices, i)
			values = append(values, v)
			return true
		})

		expectedValues := []int{3, 2, 1}
		expectedIndices := []int{0, 1, 2}

		if len(values) != len(expectedValues) {
			t.Fatalf("Iterate values length: got %d, want %d", len(values), len(expectedValues))
		}

		for i := range values {
			if values[i] != expectedValues[i] {
				t.Errorf("Iterate value[%d]: got %d, want %d", i, values[i], expectedValues[i])
			}
			if indices[i] != expectedIndices[i] {
				t.Errorf("Iterate index[%d]: got %d, want %d", i, indices[i], expectedIndices[i])
			}
		}
	})

	t.Run("IterateEarlyStop", func(t *testing.T) {
		b := New[int](5)
		for i := 1; i <= 5; i++ {
			b.Append(i)
		}

		var count int
		stopped := b.Iterate(func(i, v int) bool {
			count++
			return count < 3 // Stop after 3 iterations
		})

		if stopped {
			t.Error("Iterate should return false when stopped early")
		}

		if count != 3 {
			t.Errorf("Iterate count: got %d, want 3", count)
		}
	})

	t.Run("IterateAfterOverflow", func(t *testing.T) {
		b := New[int](3)
		// Fill beyond capacity
		for i := 1; i <= 5; i++ {
			b.Append(i)
		}

		var values []int
		b.Iterate(func(i, v int) bool {
			values = append(values, v)
			return true
		})

		// Should contain the 3 most recent values
		expectedValues := []int{5, 4, 3}

		if len(values) != len(expectedValues) {
			t.Fatalf("Values length after overflow: got %d, want %d", len(values), len(expectedValues))
		}

		for i := range values {
			if values[i] != expectedValues[i] {
				t.Errorf("Value[%d] after overflow: got %d, want %d", i, values[i], expectedValues[i])
			}
		}
	})

	t.Run("FloatBuffer", func(t *testing.T) {
		b := New[float64](10)
		b.Append(1.5)
		b.Append(2.7)
		b.Append(3.14)

		if b.Last(0) != 3.14 {
			t.Errorf("Last(0) for float: got %v, want 3.14", b.Last(0))
		}
	})

	t.Run("SetLast", func(t *testing.T) {
		b := New[int](3)
		b.Append(1)
		b.Append(2)
		b.Append(3)

		b.SetLast(0, 30)
		if b.Last(0) != 30 {
			t.Errorf("Last(0) after SetLast(0, 30): got %d, want 30", b.Last(0))
		}

		b.SetLast(2, 10)
		if b.Last(2) != 10 {
			t.Errorf("Last(2) after SetLast(2, 10): got %d, want 10", b.Last(2))
		}

		// Middle element
		b.SetLast(1, 20)
		if b.Last(1) != 20 {
			t.Errorf("Last(1) after SetLast(1, 20): got %d, want 20", b.Last(1))
		}
	})

	t.Run("SetLastOutOfRange", func(t *testing.T) {
		b := New[int](3)
		b.Append(1)
		b.Append(2)

		// Should be no-op for out of range
		b.SetLast(-1, 99)
		b.SetLast(2, 99)
		b.SetLast(100, 99)

		if b.Last(0) != 2 || b.Last(1) != 1 {
			t.Error("SetLast should be no-op for out of range indices")
		}
	})

	t.Run("SetLastAfterOverflow", func(t *testing.T) {
		b := New[int](3)
		for i := 1; i <= 5; i++ {
			b.Append(i)
		}
		// Buffer now contains [5, 4, 3] (most recent first)

		b.SetLast(0, 50)
		b.SetLast(2, 30)

		if b.Last(0) != 50 {
			t.Errorf("Last(0) after overflow SetLast: got %d, want 50", b.Last(0))
		}
		if b.Last(2) != 30 {
			t.Errorf("Last(2) after overflow SetLast: got %d, want 30", b.Last(2))
		}
	})
}