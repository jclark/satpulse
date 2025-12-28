// Package circbuf provides a generic circular buffer implementation.
package circbuf

// Buffer is a generic circular buffer that maintains a sliding window of samples.
// It is used for maintaining recent history for outlier detection algorithms
// and statistics calculations.
type Buffer[T any] struct {
	buf  []T
	end  int
	full bool
}

// New creates a new Buffer with the specified capacity.
// The size must be positive, otherwise it panics.
func New[T any](size int) *Buffer[T] {
	if size <= 0 {
		panic("size must be positive")
	}
	return &Buffer[T]{
		buf: make([]T, size),
	}
}

// Clear removes all elements from the buffer.
func (b *Buffer[T]) Clear() {
	b.end = 0
	b.full = false
}

// Len returns the number of elements currently in the buffer.
func (b *Buffer[T]) Len() int {
	if b.full {
		return len(b.buf)
	}
	return b.end
}

// Cap returns the maximum capacity of the buffer.
func (b *Buffer[T]) Cap() int {
	return len(b.buf)
}

// Append adds a new element to the buffer.
// If the buffer is full, the oldest element is overwritten.
func (b *Buffer[T]) Append(v T) {
	if b.end == len(b.buf) {
		b.end = 0
		b.full = true
	}
	b.buf[b.end] = v
	b.end++
}

// Last returns the ith most recently added value (0 = most recent).
// It returns a zero value if there was no such value.
func (b *Buffer[T]) Last(i int) T {
	if i < 0 || i >= b.Len() {
		var zero T
		return zero
	}
	j := b.end - 1 - i
	if j < 0 {
		j += len(b.buf)
	}
	return b.buf[j]
}

// Iterate iterates over the buffer, most recently added first.
// The yield function receives the index (0 = most recent) and value.
// If yield returns false, iteration stops early and Iterate returns false.
func (b *Buffer[T]) Iterate(yield func(int, T) bool) bool {
	j := b.end - 1
	for i := 0; i < len(b.buf); i++ {
		if j < 0 {
			if !b.full {
				break
			}
			j = len(b.buf) - 1
		}
		if !yield(i, b.buf[j]) {
			return false
		}
		j--
	}
	return true
}