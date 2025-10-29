// Package medwin provides an efficient data structure for computing the median
// of a fixed-size moving window. It uses two parallel arrays: a circular buffer
// storing values in arrival order, and a small index array kept sorted by value.
// This avoids duplicating large values while keeping median queries O(1).
package medwin

import (
	"cmp"
	"time"
)

// Window maintains a fixed-size moving window of ordered values and efficiently
// computes their median. Values are stored in a circular buffer in arrival order,
// with a separate array of indices kept sorted by value for O(1) median access.
// The capacity is fixed at creation time and cannot be changed.
type Window[T cmp.Ordered] struct {
	values  []T     // circular buffer: values in arrival order, len(values) is capacity
	indices []uint8 // indices into values[], kept sorted by values[i], len(indices) is current count
	head    int     // index of next insertion in values
}

// New creates a Window with the specified capacity.
// Capacity must be positive and cannot exceed 255.
func New[T cmp.Ordered](capacity int) *Window[T] {
	if capacity <= 0 {
		panic("medwin: capacity must be positive")
	}
	if capacity > 255 {
		panic("medwin: capacity cannot exceed 255")
	}
	return &Window[T]{
		values:  make([]T, capacity),
		indices: make([]uint8, 0, capacity),
	}
}

// Add inserts a value into the window. If the window is at capacity,
// the oldest value is automatically removed.
func (w *Window[T]) Add(v T) {
	cap := len(w.values)
	if len(w.indices) < cap {
		// Window not yet full
		w.values[w.head] = v
		w.insertIndex(uint8(w.head))
		w.head = (w.head + 1) % cap
	} else {
		// Window full, replace oldest
		w.removeIndex(uint8(w.head))
		w.values[w.head] = v
		w.insertIndex(uint8(w.head))
		w.head = (w.head + 1) % cap
	}
}

// Median returns the median of values in the window.
// For an even number of values, returns the average of the two middle values.
// Returns the zero value if the window is empty.
func (w *Window[T]) Median() T {
	n := len(w.indices)
	if n == 0 {
		var zero T
		return zero
	}
	mid := n / 2
	if n%2 != 0 {
		return w.values[w.indices[mid]]
	}
	// For even count, return average of two middle values
	// We use interface{} to work around Go's limitation with generic arithmetic
	a := w.values[w.indices[mid-1]]
	b := w.values[w.indices[mid]]
	return average(a, b)
}

// average computes (a+b)/2 for ordered types
// Uses any to work around generic arithmetic limitations
func average[T cmp.Ordered](a, b T) T {
	aAny, bAny := any(a), any(b)

	// Handle different numeric types
	switch aVal := aAny.(type) {
	case int:
		return any((aVal + bAny.(int)) / 2).(T)
	case int64:
		return any((aVal + bAny.(int64)) / 2).(T)
	case float64:
		return any((aVal + bAny.(float64)) / 2.0).(T)
	case float32:
		return any((aVal + bAny.(float32)) / 2.0).(T)
	case time.Duration:
		return any((aVal + bAny.(time.Duration)) / 2).(T)
	default:
		// For other types, just return the first value
		// This shouldn't happen with cmp.Ordered types we care about
		return a
	}
}

// Len returns the current number of values in the window.
func (w *Window[T]) Len() int {
	return len(w.indices)
}

// Cap returns the maximum capacity of the window.
func (w *Window[T]) Cap() int {
	return len(w.values)
}

// insertIndex inserts idx into indices at the position where values[idx]
// would maintain sorted order
func (w *Window[T]) insertIndex(idx uint8) {
	v := w.values[idx]
	// Binary search for insertion position
	pos := w.searchIndex(v, func(a, b T) bool { return a <= b })
	// Insert idx at position pos
	w.indices = append(w.indices, 0)
	copy(w.indices[pos+1:], w.indices[pos:])
	w.indices[pos] = idx
}

// removeIndex removes idx from indices
func (w *Window[T]) removeIndex(idx uint8) {
	// Linear search to find idx in indices array
	pos := -1
	for i, v := range w.indices {
		if v == idx {
			pos = i
			break
		}
	}
	if pos == -1 {
		panic("medwin: removeIndex called on index not in window")
	}
	// Remove from indices
	copy(w.indices[pos:], w.indices[pos+1:])
	w.indices = w.indices[:len(w.indices)-1]
}

// searchIndex performs binary search to find insertion position for value v
func (w *Window[T]) searchIndex(v T, cmp func(T, T) bool) int {
	left, right := 0, len(w.indices)
	for left < right {
		mid := (left + right) / 2
		if cmp(w.values[w.indices[mid]], v) {
			left = mid + 1
		} else {
			right = mid
		}
	}
	return left
}
