// Package medwin provides an efficient data structure for computing the median
// of a fixed-size moving window. It uses two parallel arrays: a circular buffer
// storing values in arrival order, and a small index array kept sorted by value.
// This avoids duplicating large values while keeping median queries O(1).
package medwin

import (
	"cmp"
	"slices"
	"time"
)

// Value is a constraint for types that can be used in median calculations.
// Supports 64-bit integers, 64-bit floats, and time.Duration (which is int64).
type Value interface {
	int64 | float64 | time.Duration
}

// Window maintains a fixed-size moving window of numeric values and efficiently
// computes their median. Values are stored in a circular buffer in arrival order,
// with a separate array of indices kept sorted by value for O(1) median access.
// The capacity is fixed at creation time and cannot be changed.
type Window[T Value] struct {
	values  []T     // circular buffer: values in arrival order, len(values) is capacity
	indices []uint8 // indices into values[], kept sorted by values[i], len(indices) is current count
	head    int     // index of next insertion in values
}

// New creates a Window with the specified capacity.
// Capacity must be positive and cannot exceed 255.
func New[T Value](capacity int) *Window[T] {
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
// For odd-sized windows, returns the middle value.
// For even-sized windows, returns the average of the two middle values.
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
	// Even count: average of two middle values
	a := w.values[w.indices[mid-1]]
	b := w.values[w.indices[mid]]
	return T((float64(a) + float64(b)) / 2)
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
	pos, _ := slices.BinarySearchFunc(w.indices, v, func(i uint8, target T) int {
		return cmp.Compare(w.values[i], target)
	})
	// Insert idx at position pos
	w.indices = slices.Insert(w.indices, pos, idx)
}

// removeIndex removes idx from indices
func (w *Window[T]) removeIndex(idx uint8) {
	// Linear search to find idx in indices array
	// Note: We can't use binary search here because indices are sorted by VALUES,
	// not by the index numbers themselves. This is O(n) but n ≤ 255.
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
	w.indices = slices.Delete(w.indices, pos, pos+1)
}
