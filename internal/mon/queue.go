package mon

type Queue[T any] struct {
	buf        []T
	start, end int
	maxLen     int
}

func NewQueue[T any](maxLen int) *Queue[T] {
	buf := make([]T, maxLen*2)
	return &Queue[T]{
		buf:    buf,
		maxLen: maxLen,
	}
}

func (q *Queue[T]) Clear() {
	q.start = 0
	q.end = 0
}

func (q *Queue[T]) Len() int {
	return q.end - q.start
}

func (q *Queue[T]) Append(f T) {
	if q.end == len(q.buf) {
		copy(q.buf, q.Slice())
		q.end -= q.start
		q.start = 0
	}
	q.buf[q.end] = f
	if q.end-q.start == q.maxLen {
		q.start++
	}
	q.end++
}

func (q *Queue[T]) Slice() []T {
	return q.buf[q.start:q.end]
}

func (q *Queue[T]) LastN(n int) []T {
	start := q.end - n
	if start < q.start {
		start = q.start
	}
	return q.buf[start:q.end]
}
