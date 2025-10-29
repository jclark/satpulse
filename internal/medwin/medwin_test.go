package medwin

import (
	"cmp"
	"slices"
	"testing"
	"time"
)

func TestWindow_BasicInt(t *testing.T) {
	w := New[int64](5)

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

	// Add second and third values
	w.Add(5)
	w.Add(15)
	// Sorted: [5, 10, 15], median is 10
	if got := w.Median(); got != 10 {
		t.Errorf("median([10,5,15]) = %d, want 10", got)
	}

	if w.Len() != 3 {
		t.Errorf("len should be 3, got %d", w.Len())
	}
}

func TestWindow_FillAndWrap(t *testing.T) {
	w := New[int64](3)

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
	w := New[int64](4)

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
	w := New[int64](5)

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
	w := New[int64](5)

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
	w := New[int64](5)

	// Median of empty window should be zero value
	if got := w.Median(); got != 0 {
		t.Errorf("median of empty window = %d, want 0", got)
	}
}

func TestWindow_SingleElement(t *testing.T) {
	w := New[int64](1)

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
	w := New[int64](5)

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
		w.Add(int64(i))
		if i >= 4 {
			want := int64(i - 2) // median of [i-4, i-3, i-2, i-1, i]
			if got := w.Median(); got != want {
				t.Errorf("after adding %d: median = %d, want %d", i, got, want)
			}
		}
	}
}

func TestWindow_NegativeValues(t *testing.T) {
	w := New[int64](5)

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

func TestWindow_MaxInt64Overflow(t *testing.T) {
	// Test that we don't overflow when averaging values near MaxInt64
	w := New[int64](2)

	w.Add(9223372036854775806) // MaxInt64 - 1
	w.Add(9223372036854775807) // MaxInt64

	// Average should be MaxInt64 - 0.5, which rounds down to MaxInt64 - 1
	want := int64(9223372036854775806)
	if got := w.Median(); got != want {
		t.Errorf("median of two MaxInt64-adjacent values = %d, want %d", got, want)
	}

	// Test with two MaxInt64 values
	w2 := New[int64](2)
	w2.Add(9223372036854775807)
	w2.Add(9223372036854775807)

	want2 := int64(9223372036854775807)
	if got := w2.Median(); got != want2 {
		t.Errorf("median of two MaxInt64 values = %d, want %d", got, want2)
	}
}

func TestWindow_MinMaxInt64Overflow(t *testing.T) {
	// Test that we handle opposite extremes (MinInt64 and MaxInt64) correctly
	// This is the critical case where b-a would overflow
	w := New[int64](2)

	w.Add(-9223372036854775808) // MinInt64
	w.Add(9223372036854775807)  // MaxInt64

	// Sum is -1, so average is -1/2 = 0 (integer division truncates towards zero)
	want := int64(0)
	if got := w.Median(); got != want {
		t.Errorf("median of MinInt64 and MaxInt64 = %d, want %d", got, want)
	}
}

func TestWindow_NegativeOddDifference(t *testing.T) {
	// Test that negative values with odd difference are averaged correctly
	// This case broke with a+(b-a)/2 formula
	w := New[int64](2)

	w.Add(-3)
	w.Add(-2)

	// (-3 + -2)/2 = -5/2 = -2 (truncate towards zero)
	want := int64(-2)
	if got := w.Median(); got != want {
		t.Errorf("median of -3 and -2 = %d, want %d", got, want)
	}

	// Test with a larger odd difference
	w2 := New[int64](2)
	w2.Add(-100)
	w2.Add(-97)

	want2 := int64(-98)
	if got := w2.Median(); got != want2 {
		t.Errorf("median of -100 and -97 = %d, want %d", got, want2)
	}
}

func TestAverage_Float64(t *testing.T) {
	tests := []struct {
		a, b, want float64
	}{
		{1.0, 2.0, 1.5},
		{-1.0, 1.0, 0.0},
		{-3.5, -2.5, -3.0},
		{1e100, 1e100, 1e100},
		{1e308, 1e308, 1e308}, // Would overflow with (a+b)/2
	}
	for _, tt := range tests {
		got := average(tt.a, tt.b)
		if got != tt.want {
			t.Errorf("average(%v, %v) = %v, want %v", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestAverage_Int64_OppositeSigns(t *testing.T) {
	tests := []struct {
		name       string
		a, b, want int64
	}{
		{"MinInt64 and MaxInt64", -9223372036854775808, 9223372036854775807, 0},
		{"negative and positive", -10, 10, 0},
		{"negative and zero", -5, 0, -2},
		{"zero and positive", 0, 5, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := average(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("average(%d, %d) = %d, want %d", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestAverage_Int64_SameSign(t *testing.T) {
	tests := []struct {
		name       string
		a, b, want int64
	}{
		{"negative odd difference", -3, -2, -2},
		{"negative even difference", -4, -2, -3},
		{"positive odd difference", 2, 3, 2},
		{"positive even difference", 2, 4, 3},
		{"large negative", -100, -97, -98},
		{"near MaxInt64", 9223372036854775806, 9223372036854775807, 9223372036854775806},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := average(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("average(%d, %d) = %d, want %d", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestAverage_Duration(t *testing.T) {
	tests := []struct {
		name       string
		a, b, want time.Duration
	}{
		{"opposite signs", -5 * time.Second, 3 * time.Second, -1 * time.Second},
		{"same sign odd", -3 * time.Nanosecond, -2 * time.Nanosecond, -2 * time.Nanosecond},
		{"large precision", 9000000000000000000, 9000000000000000002, 9000000000000000001},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := average(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("average(%v, %v) = %v, want %v", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestWindow_DurationPrecision(t *testing.T) {
	// Test that large time.Duration values maintain precision
	w := New[time.Duration](2)

	// Large durations near int64 max (roughly 290 years in nanoseconds)
	d1 := time.Duration(9000000000000000000) // 9e18 ns
	d2 := time.Duration(9000000000000000002) // 9e18 + 2 ns

	w.Add(d1)
	w.Add(d2)

	// Average should be d1 + 1ns, not lose precision
	want := time.Duration(9000000000000000001)
	if got := w.Median(); got != want {
		t.Errorf("median of large durations = %v, want %v (diff: %v)", got, want, got-want)
	}
}

func BenchmarkWindow_Add(b *testing.B) {
	w := New[int64](99)
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		w.Add(int64(i))
	}
}

func BenchmarkWindow_Median(b *testing.B) {
	w := New[int64](99)
	for i := 0; i < 99; i++ {
		w.Add(int64(i))
	}
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = w.Median()
	}
}

func BenchmarkWindow_AddAndMedian(b *testing.B) {
	w := New[int64](99)
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		w.Add(int64(i))
		_ = w.Median()
	}
}

// Reference implementation using naive remove+insert
func replaceIndexNaive[T Value](values []T, indices *[]uint8, oldIdx uint8, newVal T) {
	values[oldIdx] = newVal

	// Remove oldIdx
	pos := -1
	for i, v := range *indices {
		if v == oldIdx {
			pos = i
			break
		}
	}
	if pos == -1 {
		panic("oldIdx not found in naive implementation")
	}
	*indices = slices.Delete(*indices, pos, pos+1)

	// Insert oldIdx at new position
	insertPos, _ := slices.BinarySearchFunc(*indices, newVal, func(i uint8, target T) int {
		return cmp.Compare(values[i], target)
	})
	*indices = slices.Insert(*indices, insertPos, oldIdx)
}

// Helper to clone a window for testing
func (w *Window[T]) clone() *Window[T] {
	c := &Window[T]{
		values:  slices.Clone(w.values),
		indices: slices.Clone(w.indices),
		head:    w.head,
	}
	return c
}

// Helper to check if indices are properly sorted by values
func (w *Window[T]) isValidlySorted() bool {
	for i := 1; i < len(w.indices); i++ {
		if w.values[w.indices[i-1]] > w.values[w.indices[i]] {
			return false
		}
	}
	return true
}

func FuzzReplaceIndex(f *testing.F) {
	// Seed corpus with interesting cases
	f.Add(int64(5), int64(3), int64(10), int64(20), int64(30), int64(15))
	f.Add(int64(7), int64(0), int64(50), int64(40), int64(30), int64(35))
	f.Add(int64(3), int64(1), int64(10), int64(20), int64(30), int64(5))

	f.Fuzz(func(t *testing.T, cap int64, oldIdx int64, v1, v2, v3, newVal int64) {
		// Constrain inputs to valid ranges
		if cap < 3 || cap > 255 {
			return
		}
		capacity := int(cap)

		if oldIdx < 0 || oldIdx >= int64(capacity) {
			return
		}

		// Create a window and fill it
		w1 := New[int64](capacity)
		w2 := New[int64](capacity)

		// Add initial values to fill the window
		values := []int64{v1, v2, v3}
		for i := 0; i < capacity; i++ {
			val := values[i%len(values)] + int64(i)*7 // Mix it up
			w1.Add(val)
			w2.Add(val)
		}

		// Both windows should be identical now
		if !slices.Equal(w1.indices, w2.indices) {
			t.Fatal("windows diverged during setup")
		}

		// Test replaceIndex
		idx := uint8(oldIdx)

		// w1 uses optimized version
		w1.replaceIndex(idx, newVal)

		// w2 uses naive version
		replaceIndexNaive(w2.values, &w2.indices, idx, newVal)

		// Compare results
		if !slices.Equal(w1.indices, w2.indices) {
			t.Errorf("indices mismatch after replace\noptimized: %v\nnaive:     %v",
				w1.indices, w2.indices)
		}

		if !slices.Equal(w1.values, w2.values) {
			t.Errorf("values mismatch after replace")
		}

		// Verify both are properly sorted
		if !w1.isValidlySorted() {
			t.Error("optimized version produced unsorted indices")
		}
		if !w2.isValidlySorted() {
			t.Error("naive version produced unsorted indices")
		}

		// Verify medians match
		if w1.Median() != w2.Median() {
			t.Errorf("median mismatch: optimized=%d, naive=%d", w1.Median(), w2.Median())
		}
	})
}

func TestReplaceIndex_EdgeCases(t *testing.T) {
	tests := []struct {
		name      string
		capacity  int
		initial   []int64
		oldIdx    uint8
		newVal    int64
	}{
		{
			name:     "replace smallest with larger",
			capacity: 5,
			initial:  []int64{10, 20, 30, 40, 50},
			oldIdx:   0,
			newVal:   25,
		},
		{
			name:     "replace largest with smaller",
			capacity: 5,
			initial:  []int64{10, 20, 30, 40, 50},
			oldIdx:   4,
			newVal:   15,
		},
		{
			name:     "replace middle with smallest",
			capacity: 5,
			initial:  []int64{10, 20, 30, 40, 50},
			oldIdx:   2,
			newVal:   5,
		},
		{
			name:     "replace middle with largest",
			capacity: 5,
			initial:  []int64{10, 20, 30, 40, 50},
			oldIdx:   2,
			newVal:   55,
		},
		{
			name:     "replace with same value",
			capacity: 3,
			initial:  []int64{10, 20, 30},
			oldIdx:   1,
			newVal:   20,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := New[int64](tt.capacity)
			for _, v := range tt.initial {
				w.Add(v)
			}

			// Clone for comparison
			w2 := w.clone()

			// Apply optimized version
			w.replaceIndex(tt.oldIdx, tt.newVal)

			// Apply naive version
			replaceIndexNaive(w2.values, &w2.indices, tt.oldIdx, tt.newVal)

			// Compare
			if !slices.Equal(w.indices, w2.indices) {
				t.Errorf("indices mismatch\noptimized: %v\nnaive:     %v",
					w.indices, w2.indices)
			}

			if !w.isValidlySorted() {
				t.Error("result not properly sorted")
			}
		})
	}
}
