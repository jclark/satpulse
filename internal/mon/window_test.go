package mon

import "testing"

func TestWindow(t *testing.T) {
	// just tests unused functions at the moment
	w := newWindow[float64](sampleWindowSize)
	if w.capacity() != sampleWindowSize {
		t.Errorf("capacity: got %d, want %d", w.capacity(), sampleWindowSize)
	}
	w.append(1)
	w.append(17)
	w.append(42)
	if w.last(0) != 42 {
		t.Errorf("last: got %v, want 42", w.last(0))
	}
}
