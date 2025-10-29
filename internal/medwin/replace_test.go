package medwin

import (
	"cmp"
	"slices"
	"testing"
)

// Reference implementation using naive remove+insert
func replaceIndexNaive[T Value](values []T, indices []uint8, oldIdx uint8, newVal T) {
	values[oldIdx] = newVal

	// Remove oldIdx
	pos := -1
	for i, v := range indices {
		if v == oldIdx {
			pos = i
			break
		}
	}
	if pos == -1 {
		panic("oldIdx not found in naive implementation")
	}
	indices = slices.Delete(indices, pos, pos+1)

	// Insert oldIdx at new position
	insertPos, _ := slices.BinarySearchFunc(indices, newVal, func(i uint8, target T) int {
		return cmp.Compare(values[i], target)
	})
	slices.Insert(indices, insertPos, oldIdx)
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
		// Remove
		pos := -1
		for i, v := range w2.indices {
			if v == idx {
				pos = i
				break
			}
		}
		if pos == -1 {
			t.Fatal("oldIdx not found in naive implementation")
		}
		w2.indices = slices.Delete(w2.indices, pos, pos+1)

		// Update value
		w2.values[idx] = newVal

		// Insert
		insertPos, _ := slices.BinarySearchFunc(w2.indices, newVal, func(i uint8, target int64) int {
			return cmp.Compare(w2.values[i], target)
		})
		w2.indices = slices.Insert(w2.indices, insertPos, idx)

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
			pos := -1
			for i, v := range w2.indices {
				if v == tt.oldIdx {
					pos = i
					break
				}
			}
			w2.indices = slices.Delete(w2.indices, pos, pos+1)
			w2.values[tt.oldIdx] = tt.newVal
			insertPos, _ := slices.BinarySearchFunc(w2.indices, tt.newVal, func(i uint8, target int64) int {
				return cmp.Compare(w2.values[i], target)
			})
			w2.indices = slices.Insert(w2.indices, insertPos, tt.oldIdx)

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
