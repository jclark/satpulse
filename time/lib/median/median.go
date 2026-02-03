// Package median provides an efficient data structure for computing the median
// of a fixed-size moving window. It uses two parallel arrays: a circular buffer
// storing values in arrival order, and a small index array kept sorted by value.
// This avoids duplicating large values while keeping median queries O(1).
package median

import (
	"cmp"
	"iter"
	"slices"
	"time"
)

// Value is a constraint for types that can be used in median calculations.
// Supports 64-bit integers, 64-bit floats, and time.Duration (which is int64).
type Value interface {
	int64 | float64 | time.Duration
}

// winIndex is the type used for indexing into the window.
// Change this type alias to switch between different capacity limits:
//   - uint8:  max capacity 256,   smaller memory footprint
//   - uint16: max capacity 65536, larger windows supported
//
// Update MaxCapacity to match.
type winIndex = uint16

// MaxCapacity is the maximum allowed window capacity based on winIndex type.
// Set to 256 for uint8, or 65536 for uint16.
const MaxCapacity = 65536

// Window maintains a fixed-size moving window of numeric values and efficiently
// computes their median. Values are stored in a circular buffer in arrival order,
// with a separate array of indices kept sorted by value for O(1) median access.
// The capacity is fixed at creation time and cannot be changed.
type Window[T Value] struct {
	values  []T        // circular buffer: values in arrival order, len(values) is capacity
	indices []winIndex // indices into values[], kept sorted by values[i], len(indices) is current count
	head    int        // index of next insertion in values
}

// New creates a Window with the specified capacity.
// Capacity must be positive and cannot exceed MaxCapacity.
func New[T Value](capacity int) *Window[T] {
	if capacity <= 0 {
		panic("median: capacity must be positive")
	}
	if capacity > MaxCapacity {
		panic("median: capacity cannot exceed MaxCapacity")
	}
	return &Window[T]{
		values:  make([]T, capacity),
		indices: make([]winIndex, 0, capacity),
	}
}

// Add inserts a value into the window. If the window is at capacity,
// the oldest value is automatically removed.
func (w *Window[T]) Add(v T) {
	if len(w.indices) >= len(w.values) {
		w.replaceIndex(winIndex(w.head), v)
	} else {
		w.values[w.head] = v
		w.insertIndex(winIndex(w.head))
	}
	w.head = (w.head + 1) % len(w.values)
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
	return average(w.values[w.indices[mid-1]], w.values[w.indices[mid]])
}

// average returns the average of two values without overflow.
// Assumes a <= b.
func average[T Value](a, b T) T {
	// For float64, use standard formula (precise for common small values)
	if _, ok := any(a).(float64); ok {
		return (a + b) / 2
	}
	// For integer types (int64, time.Duration):
	// When opposite signs: (a+b)/2 is safe (no overflow)
	if a < 0 && b >= 0 {
		return (a + b) / 2
	}
	// When same sign: use a/2 + b/2 + (a%2 + b%2)/2 to avoid overflow
	// and match (a+b)/2 rounding exactly
	return a/2 + b/2 + T((int64(a)%2+int64(b)%2)/2)
}

// Median computes the median of values from an iterator by collecting and sorting them.
// For odd-length sequences, returns the middle value.
// For even-length sequences, returns the average of the two middle values.
// Returns the zero value if the sequence is empty.
func Median[T Value](seq iter.Seq[T]) T {
	values := make([]T, 0, 16) // reasonable initial capacity
	for v := range seq {
		values = append(values, v)
	}
	n := len(values)
	if n == 0 {
		var zero T
		return zero
	}
	slices.Sort(values)
	mid := n / 2
	if n%2 != 0 {
		return values[mid]
	}
	return average(values[mid-1], values[mid])
}

// Len returns the current number of values in the window.
func (w *Window[T]) Len() int {
	return len(w.indices)
}

// Cap returns the maximum capacity of the window.
func (w *Window[T]) Cap() int {
	return len(w.values)
}

// Last returns the most recently added value.
// Panics if the window is empty.
func (w *Window[T]) Last() T {
	if len(w.indices) == 0 {
		panic("median: Last() called on empty window")
	}
	// head points to the next insertion position, so the last inserted value
	// is at (head - 1) mod capacity
	lastIdx := (w.head - 1 + len(w.values)) % len(w.values)
	return w.values[lastIdx]
}

// Iterate calls yield for each value in the window, from newest to oldest.
// If yield returns false, iteration stops and Iterate returns false.
// Otherwise, Iterate returns true after visiting all values.
func (w *Window[T]) Iterate(yield func(int, T) bool) bool {
	for i := range len(w.indices) {
		idx := (w.head - 1 - i + len(w.values)) % len(w.values)
		if !yield(i, w.values[idx]) {
			return false
		}
	}
	return true
}

// insertIndex inserts idx into indices at the position where values[idx]
// would maintain sorted order
func (w *Window[T]) insertIndex(idx winIndex) {
	v := w.values[idx]
	// Binary search for insertion position
	pos, _ := slices.BinarySearchFunc(w.indices, v, func(i winIndex, target T) int {
		return cmp.Compare(w.values[i], target)
	})
	// Insert idx at position pos
	w.indices = slices.Insert(w.indices, pos, idx)
}

// replaceIndex repositions idx in the sorted indices array after updating its value.
// This is called when replacing the oldest value in a full window.
func (w *Window[T]) replaceIndex(idx winIndex, newVal T) {
	w.values[idx] = newVal
	// Remove idx from its current position
	pos := slices.Index(w.indices, idx)
	if pos < 0 {
		panic("median: index not found in replaceIndex")
	}
	w.indices = slices.Delete(w.indices, pos, pos+1)
	// Insert idx at new position
	insertPos, _ := slices.BinarySearchFunc(w.indices, newVal, func(i winIndex, target T) int {
		return cmp.Compare(w.values[i], target)
	})
	w.indices = slices.Insert(w.indices, insertPos, idx)
}
