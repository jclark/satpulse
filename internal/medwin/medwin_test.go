package medwin

import (
	"testing"
	"time"
)

func TestWindow_BasicInt(t *testing.T) {
	w := New[int](5)

	if w.Len() != 0 {
		t.Errorf("new window should have len 0, got %d", w.Len())
	}
	if w.Cap() != 5 {
		t.Errorf("expected cap 5, got %d", w.Cap())
	}

	// Add first value
	w.Add(10)
	if got := w.Median(); got != 10 {
		t.Errorf("median([10]) = %d, want 10", got)
	}

	// Add second value
	w.Add(5)
	if got := w.Median(); got != 7 {
		t.Errorf("median([10,5]) = %d, want 7", got)
	}

	// Add third value (odd count)
	w.Add(15)
	if got := w.Median(); got != 10 {
		t.Errorf("median([10,5,15]) = %d, want 10", got)
	}

	if w.Len() != 3 {
		t.Errorf("len should be 3, got %d", w.Len())
	}
}

func TestWindow_FillAndWrap(t *testing.T) {
	w := New[int](3)

	// Fill window: [1, 2, 3]
	w.Add(1)
	w.Add(2)
	w.Add(3)

	if got := w.Median(); got != 2 {
		t.Errorf("median([1,2,3]) = %d, want 2", got)
	}

	// Add 4th value, oldest (1) is removed: [2, 3, 4]
	w.Add(4)
	if got := w.Median(); got != 3 {
		t.Errorf("median([2,3,4]) = %d, want 3", got)
	}

	// Add 5th value, oldest (2) is removed: [3, 4, 5]
	w.Add(5)
	if got := w.Median(); got != 4 {
		t.Errorf("median([3,4,5]) = %d, want 4", got)
	}

	if w.Len() != 3 {
		t.Errorf("len should remain 3, got %d", w.Len())
	}
}

func TestWindow_Float64(t *testing.T) {
	w := New[float64](3)

	w.Add(1.5)
	w.Add(2.5)
	w.Add(3.5)

	if got := w.Median(); got != 2.5 {
		t.Errorf("median([1.5,2.5,3.5]) = %f, want 2.5", got)
	}

	// Add 4.5, removing 1.5: [2.5, 3.5, 4.5]
	w.Add(4.5)
	if got := w.Median(); got != 3.5 {
		t.Errorf("median([2.5,3.5,4.5]) = %f, want 3.5", got)
	}
}

func TestWindow_Duration(t *testing.T) {
	w := New[time.Duration](3)

	w.Add(100 * time.Nanosecond)
	w.Add(200 * time.Nanosecond)
	w.Add(300 * time.Nanosecond)

	want := 200 * time.Nanosecond
	if got := w.Median(); got != want {
		t.Errorf("median = %v, want %v", got, want)
	}
}

func TestWindow_EvenSize(t *testing.T) {
	w := New[int](4)

	w.Add(10)
	w.Add(20)
	w.Add(30)
	w.Add(40)

	// Even count: average of middle two (20, 30) = 25
	if got := w.Median(); got != 25 {
		t.Errorf("median([10,20,30,40]) = %d, want 25", got)
	}
}

func TestWindow_DuplicateValues(t *testing.T) {
	w := New[int](5)

	w.Add(5)
	w.Add(5)
	w.Add(5)

	if got := w.Median(); got != 5 {
		t.Errorf("median([5,5,5]) = %d, want 5", got)
	}

	w.Add(10)
	w.Add(10)

	// Sorted: [5,5,5,10,10], median is 5
	if got := w.Median(); got != 5 {
		t.Errorf("median([5,5,5,10,10]) = %d, want 5", got)
	}

	// Add 10, remove first 5: [5,5,10,10,10], median is 10
	w.Add(10)
	if got := w.Median(); got != 10 {
		t.Errorf("median([5,5,10,10,10]) = %d, want 10", got)
	}
}

func TestWindow_UnsortedInput(t *testing.T) {
	w := New[int](5)

	// Add values in random order
	w.Add(50)
	w.Add(10)
	w.Add(30)
	w.Add(20)
	w.Add(40)

	// Sorted: [10,20,30,40,50], median is 30
	if got := w.Median(); got != 30 {
		t.Errorf("median = %d, want 30", got)
	}
}

func TestWindow_Empty(t *testing.T) {
	w := New[int](5)

	// Median of empty window should be zero value
	if got := w.Median(); got != 0 {
		t.Errorf("median of empty window = %d, want 0", got)
	}
}

func TestWindow_SingleElement(t *testing.T) {
	w := New[int](1)

	w.Add(42)
	if got := w.Median(); got != 42 {
		t.Errorf("median([42]) = %d, want 42", got)
	}

	// Add another, first is removed
	w.Add(100)
	if got := w.Median(); got != 100 {
		t.Errorf("median([100]) = %d, want 100", got)
	}
}

func TestWindow_LongSequence(t *testing.T) {
	w := New[int](5)

	// Add sequence 0..19, checking median at each step after window is full
	// Window contents → median
	// [0] → 0
	// [0,1] → 0
	// [0,1,2] → 1
	// [0,1,2,3] → 1
	// [0,1,2,3,4] → 2
	// [1,2,3,4,5] → 3
	// [2,3,4,5,6] → 4
	// [3,4,5,6,7] → 5
	// etc.

	for i := 0; i < 20; i++ {
		w.Add(i)
		if i >= 4 {
			want := i - 2 // median of [i-4, i-3, i-2, i-1, i]
			if got := w.Median(); got != want {
				t.Errorf("after adding %d: median = %d, want %d", i, got, want)
			}
		}
	}
}

func TestWindow_NegativeValues(t *testing.T) {
	w := New[int](5)

	w.Add(-10)
	w.Add(-5)
	w.Add(0)
	w.Add(5)
	w.Add(10)

	// Sorted: [-10,-5,0,5,10], median is 0
	if got := w.Median(); got != 0 {
		t.Errorf("median = %d, want 0", got)
	}
}

func BenchmarkWindow_Add(b *testing.B) {
	w := New[int](100)
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		w.Add(i)
	}
}

func BenchmarkWindow_Median(b *testing.B) {
	w := New[int](100)
	for i := 0; i < 100; i++ {
		w.Add(i)
	}
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = w.Median()
	}
}

func BenchmarkWindow_AddAndMedian(b *testing.B) {
	w := New[int](100)
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		w.Add(i)
		_ = w.Median()
	}
}
