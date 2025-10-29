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
	if len(w.indices) >= len(w.values) {
		w.replaceIndex(uint8(w.head), v)
	} else {
		w.values[w.head] = v
		w.insertIndex(uint8(w.head))
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
func average[T Value](a, b T) T {
	// For float64, direct average is fine (no overflow concerns)
	if _, ok := any(a).(float64); ok {
		return (a + b) / 2
	}
	// For integer types (int64, time.Duration): convert to int64 for modulo
	var aInt, bInt int64
	switch v := any(a).(type) {
	case int64:
		aInt = v
		bInt = any(b).(int64)
	case time.Duration:
		aInt = int64(v)
		bInt = int64(any(b).(time.Duration))
	}
	// When opposite signs: (a+b)/2 is safe (no overflow)
	if aInt < 0 && bInt >= 0 {
		return (a + b) / 2
	}
	// When same sign: use a/2 + b/2 + (a%2 + b%2)/2 to avoid overflow
	// and match (a+b)/2 rounding exactly
	return T(aInt/2 + bInt/2 + (aInt%2+bInt%2)/2)
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

// replaceIndex repositions idx in the sorted indices array after updating its value.
// This is called when replacing the oldest value in a full window.
//
// idx: the circular buffer slot being updated (e.g., values[3])
// newVal: the new value being stored at values[idx]
//
// The index idx is currently somewhere in the sorted indices array. We need to:
// 1. Remove idx from its current position
// 2. Insert idx at the new position where values[idx]=newVal belongs
//
// This is done in a single pass for efficiency:
// - Scan from high to low looking for either idx or the insertion position
// - Shift elements in one direction to accomplish both remove and insert
func (w *Window[T]) replaceIndex(idx uint8, newVal T) {
	// Update the value first so subsequent comparisons use the new value
	w.values[idx] = newVal
	n := len(w.indices)

	// Scan from high to low looking for either idx or the insertion position for newVal
	for i := n - 1; i >= 0; i-- {
		if w.indices[i] == idx {
			// Found idx first - this means newVal belongs at or before position i
			// Shift elements right while searching backward for the insertion position
			// Use >= to insert BEFORE equal values (matching BinarySearchFunc behavior)
			pos := i
			for pos > 0 && w.values[w.indices[pos-1]] >= newVal {
				w.indices[pos] = w.indices[pos-1]
				pos--
			}
			w.indices[pos] = idx
			return
		}

		if w.values[w.indices[i]] < newVal {
			// Found insertion position first (at i+1) - this means idx must be at or before i
			// Shift elements left as we search backward for idx (single pass)
			saved := w.indices[i]  // Save the value that will be overwritten
			for j := i - 1; j >= 0; j-- {
				if w.indices[j] == idx {
					// Found idx! Place saved value at j, idx at i
					w.indices[j] = saved
					w.indices[i] = idx
					return
				}
				// Haven't found idx yet. Shift: save current, write previous saved
				temp := w.indices[j]
				w.indices[j] = saved
				saved = temp
			}
			panic("medwin: idx not found before insertion position - internal error")
		}
	}

	// Should never reach here - idx must be in the indices array
	panic("medwin: replaceIndex failed to find idx - internal error")
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
