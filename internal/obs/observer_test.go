package obs

import (
	"testing"
	"time"

	"github.com/jclark/satpulse/internal/gpsprot"
	"github.com/jclark/satpulse/internal/mon"
)

type mockObserver struct {
	gpsprot.DefaultHandler
	sampleCount  int
	releaseCount int
	timeCount    int
}

func (m *mockObserver) Sample(data mon.SampleData) {
	m.sampleCount++
}

func (m *mockObserver) Release() {
	m.releaseCount++
}

func (m *mockObserver) Time(msg *gpsprot.TimeMsg, tRead time.Time) {
	m.timeCount++
}

func TestMultiObserver_Sample(t *testing.T) {
	mock1 := &mockObserver{}
	mock2 := &mockObserver{}
	multi := NewMultiObserver(mock1, mock2)

	multi.Sample(mon.SampleData{})

	if mock1.sampleCount != 1 {
		t.Errorf("Expected Sample count 1 on mock1, got %d", mock1.sampleCount)
	}
	if mock2.sampleCount != 1 {
		t.Errorf("Expected Sample count 1 on mock2, got %d", mock2.sampleCount)
	}
}

func TestMultiObserver_Release(t *testing.T) {
	mock1 := &mockObserver{}
	mock2 := &mockObserver{}
	multi := NewMultiObserver(mock1, mock2)

	multi.Release()

	if mock1.releaseCount != 1 {
		t.Errorf("Expected Release count 1 on mock1, got %d", mock1.releaseCount)
	}
	if mock2.releaseCount != 1 {
		t.Errorf("Expected Release count 1 on mock2, got %d", mock2.releaseCount)
	}
}

func TestMultiObserver_Time(t *testing.T) {
	mock1 := &mockObserver{}
	mock2 := &mockObserver{}
	multi := NewMultiObserver(mock1, mock2)

	multi.Time(&gpsprot.TimeMsg{}, time.Now())

	if mock1.timeCount != 1 {
		t.Errorf("Expected Time count 1 on mock1, got %d", mock1.timeCount)
	}
	if mock2.timeCount != 1 {
		t.Errorf("Expected Time count 1 on mock2, got %d", mock2.timeCount)
	}
}