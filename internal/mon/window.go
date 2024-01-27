package mon

type window[T any] struct {
	buf  []T
	end  int
	full bool
}

func newWindow[T any](size int) *window[T] {
	if size <= 0 {
		panic("size must be positive")
	}
	return &window[T]{
		buf: make([]T, size),
	}
}

func (w *window[T]) length() int {
	if w.full {
		return len(w.buf)
	}
	return w.end
}

func (w *window[T]) capacity() int {
	return len(w.buf)
}

func (w *window[T]) append(v T) {
	if w.end == len(w.buf) {
		w.end = 0
		w.full = true
	}
	w.buf[w.end] = v
	w.end++
}

// last returns the ith most recently added value.
// It returns a zero value if there was no such value
func (w *window[T]) last(i int) T {
	if i < 0 || i >= w.length() {
		var zero T
		return zero
	}
	j := w.end - 1 - i
	if j < 0 {
		j += len(w.buf)
	}
	return w.buf[j]
}

// iterate iterates over the window, most recently added first.
func (w *window[T]) iterate(yield func(int, T) bool) bool {
	j := w.end - 1
	for i := 0; i < len(w.buf); i++ {
		if j < 0 {
			if !w.full {
				break
			}
			j = len(w.buf) - 1
		}
		if !yield(i, w.buf[j]) {
			return false
		}
		j--
	}
	return true
}
