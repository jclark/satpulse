package median

import (
	"fmt"
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

	for i := range 20 {
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

func TestWindow_Last(t *testing.T) {
	w := New[int64](5)

	// Empty window should panic
	defer func() {
		if r := recover(); r == nil {
			t.Error("Last() on empty window should panic")
		}
	}()
	_ = w.Last()
}

func TestWindow_Last_Values(t *testing.T) {
	w := New[int64](5)

	// Add values and check Last() returns the most recent
	w.Add(10)
	if got := w.Last(); got != 10 {
		t.Errorf("Last() = %d, want 10", got)
	}

	w.Add(20)
	if got := w.Last(); got != 20 {
		t.Errorf("Last() = %d, want 20", got)
	}

	w.Add(30)
	if got := w.Last(); got != 30 {
		t.Errorf("Last() = %d, want 30", got)
	}

	// Fill window and wrap around
	w.Add(40)
	w.Add(50)
	if got := w.Last(); got != 50 {
		t.Errorf("Last() = %d, want 50", got)
	}

	// Add more to test wraparound
	w.Add(60)
	if got := w.Last(); got != 60 {
		t.Errorf("Last() after wraparound = %d, want 60", got)
	}
}

func TestWindow_Iterate(t *testing.T) {
	w := New[int64](5)

	// Empty window should iterate zero times
	count := 0
	w.Iterate(func(i int, v int64) bool {
		count++
		return true
	})
	if count != 0 {
		t.Errorf("Iterate on empty window called yield %d times, want 0", count)
	}

	// Add some values: 10, 20, 30
	w.Add(10)
	w.Add(20)
	w.Add(30)

	// Should iterate newest to oldest: 30, 20, 10
	expected := []int64{30, 20, 10}
	i := 0
	w.Iterate(func(idx int, v int64) bool {
		if idx != i {
			t.Errorf("index = %d, want %d", idx, i)
		}
		if v != expected[i] {
			t.Errorf("value[%d] = %d, want %d", i, v, expected[i])
		}
		i++
		return true
	})
	if i != 3 {
		t.Errorf("Iterate called yield %d times, want 3", i)
	}

	// Fill window and wraparound: 10, 20, 30, 40, 50
	w.Add(40)
	w.Add(50)
	// Then add 60, 70 which replaces 10, 20
	// Window now: 30, 40, 50, 60, 70
	w.Add(60)
	w.Add(70)

	expected = []int64{70, 60, 50, 40, 30}
	i = 0
	w.Iterate(func(idx int, v int64) bool {
		if v != expected[i] {
			t.Errorf("value[%d] = %d, want %d (after wraparound)", i, v, expected[i])
		}
		i++
		return true
	})
	if i != 5 {
		t.Errorf("Iterate called yield %d times, want 5", i)
	}

	// Test early termination
	i = 0
	result := w.Iterate(func(idx int, v int64) bool {
		i++
		return i < 3 // Stop after 3 iterations
	})
	if result {
		t.Error("Iterate should return false when yield returns false")
	}
	if i != 3 {
		t.Errorf("Early termination: visited %d values, want 3", i)
	}
}

func TestWindow_MaxCapacity(t *testing.T) {
	// Test that we can create a window at MaxCapacity
	w := New[int64](MaxCapacity)
	if w.Cap() != MaxCapacity {
		t.Errorf("window capacity = %d, want %d", w.Cap(), MaxCapacity)
	}

	// Add values and verify it works
	for i := range MaxCapacity + 100 {
		w.Add(int64(i))
	}

	// Should have exactly MaxCapacity elements
	if w.Len() != MaxCapacity {
		t.Errorf("window length = %d, want %d", w.Len(), MaxCapacity)
	}

	// Median should be reasonable
	median := w.Median()
	expected := int64(MaxCapacity/2 + 100) // Middle of the window after 100 extra adds
	if median < expected-1 || median > expected+1 {
		t.Errorf("median = %d, expected around %d", median, expected)
	}
}

func TestWindow_CapacityBoundary(t *testing.T) {
	// Test creating window at MaxCapacity (should work)
	w := New[int64](MaxCapacity)
	if w.Cap() != MaxCapacity {
		t.Errorf("failed to create window at MaxCapacity")
	}

	// Test that exceeding MaxCapacity panics
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("creating window > MaxCapacity should panic")
		}
	}()
	_ = New[int64](MaxCapacity + 1)
}

func BenchmarkWindow_Add(b *testing.B) {
	sizes := []int{5, 50, 250, 1250, 6250, 31250, 65535}
	for _, size := range sizes {
		if size > MaxCapacity {
			continue
		}
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			w := New[int64](size)
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				w.Add(int64(i))
			}
		})
	}
}

func BenchmarkWindow_Median(b *testing.B) {
	sizes := []int{5, 50, 250, 1250, 6250, 31250, 65535}
	for _, size := range sizes {
		if size > MaxCapacity {
			continue
		}
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			w := New[int64](size)
			for i := range size {
				w.Add(int64(i))
			}
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = w.Median()
			}
		})
	}
}
