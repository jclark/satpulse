// Package medwin provides an efficient data structure for computing the median
// of a fixed-size moving window. It maintains values in sorted order and uses
// binary search for O(log n) lookups with O(n) insertions/deletions. For small
// window sizes (typical: 3-100), this is simpler and more efficient than heap-based
// approaches while providing O(1) median queries.
package medwin

import (
	"cmp"
	"slices"
)

// Window maintains a fixed-size moving window of ordered values and efficiently
// computes their median. Values are kept in sorted order for O(1) median access.
// The capacity is fixed at creation time and cannot be changed.
type Window[T cmp.Ordered] struct {
	sorted []T // values in sorted order
	ring   []T // values in insertion order (circular buffer)
	head   int // index of next insertion in ring
	cap    int // maximum window size
	len    int // current number of values
}

// New creates a Window with the specified capacity.
// Capacity must be positive.
func New[T cmp.Ordered](capacity int) *Window[T] {
	if capacity <= 0 {
		panic("medwin: capacity must be positive")
	}
	return &Window[T]{
		sorted: make([]T, 0, capacity),
		ring:   make([]T, capacity),
		cap:    capacity,
	}
}

// Add inserts a value into the window. If the window is at capacity,
// the oldest value is automatically removed.
func (w *Window[T]) Add(v T) {
	if w.len < w.cap {
		// Window not yet full
		w.ring[w.head] = v
		w.head = (w.head + 1) % w.cap
		w.len++
		w.insert(v)
	} else {
		// Window full, replace oldest
		old := w.ring[w.head]
		w.ring[w.head] = v
		w.head = (w.head + 1) % w.cap
		w.remove(old)
		w.insert(v)
	}
}

// Median returns the median of values in the window.
// For an even number of values, returns the average of the two middle values.
// Returns the zero value if the window is empty.
func (w *Window[T]) Median() T {
	if w.len == 0 {
		var zero T
		return zero
	}
	mid := w.len / 2
	if w.len%2 != 0 {
		return w.sorted[mid]
	}
	return avg(w.sorted[mid-1], w.sorted[mid])
}

// Len returns the current number of values in the window.
func (w *Window[T]) Len() int {
	return w.len
}

// Cap returns the maximum capacity of the window.
func (w *Window[T]) Cap() int {
	return w.cap
}

// insert adds v to sorted in the correct position
func (w *Window[T]) insert(v T) {
	pos, _ := slices.BinarySearch(w.sorted, v)
	w.sorted = slices.Insert(w.sorted, pos, v)
}

// remove deletes the first occurrence of v from sorted
func (w *Window[T]) remove(v T) {
	pos, found := slices.BinarySearch(w.sorted, v)
	if !found {
		// Value not found - should not happen in normal operation
		// Search linearly as fallback
		for i, val := range w.sorted {
			if val == v {
				pos = i
				found = true
				break
			}
		}
		if !found {
			panic("medwin: remove called on value not in window")
		}
	}
	w.sorted = slices.Delete(w.sorted, pos, pos+1)
}

// avg computes the average of two ordered values
func avg[T cmp.Ordered](a, b T) T {
	// This works for numeric types; the addition and division
	// will be properly handled by the type's operators
	return (a + b) / 2
}
