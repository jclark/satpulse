package obs

import (
	"testing"
	"time"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/ptime"
	"github.com/jclark/satpulse/time/internal/phcsync"
)

type mockObserver struct {
	gpsprot.DefaultHandler
	sampleCount     int
	releaseCount    int
	reopenCount     int
	timeCount       int
	tickCount       int
	navEpochPVCount int
	ntpSampleCount  int
}

func (m *mockObserver) Sample(data phcsync.Sample) {
	m.sampleCount++
}

func (m *mockObserver) Tick(_ *gpsprot.TimeMsg, _ time.Time) {
	m.tickCount++
}

func (m *mockObserver) NavEpochPV(_ *gpsprot.NavEpochMsg, _ *gpsprot.PVMsgBundle, _ time.Time) {
	m.navEpochPVCount++
}

func (m *mockObserver) NTPSample(_ time.Time, _ float64, _ ptime.Time) {
	m.ntpSampleCount++
}

func (m *mockObserver) ReopenLog() {
	m.reopenCount++
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

	multi.Sample(phcsync.Sample{})

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

func TestMultiObserver_ReopenLog(t *testing.T) {
	mock1 := &mockObserver{}
	mock2 := &mockObserver{}
	multi := NewMultiObserver(mock1, mock2)

	multi.ReopenLog()

	if mock1.reopenCount != 1 {
		t.Errorf("Expected ReopenLog count 1 on mock1, got %d", mock1.reopenCount)
	}
	if mock2.reopenCount != 1 {
		t.Errorf("Expected ReopenLog count 1 on mock2, got %d", mock2.reopenCount)
	}
}

func TestMultiObserver_Tick(t *testing.T) {
	mock1 := &mockObserver{}
	mock2 := &mockObserver{}
	multi := NewMultiObserver(mock1, mock2)

	multi.Tick(&gpsprot.TimeMsg{}, time.Now())

	if mock1.tickCount != 1 {
		t.Errorf("Expected Tick count 1 on mock1, got %d", mock1.tickCount)
	}
	if mock2.tickCount != 1 {
		t.Errorf("Expected Tick count 1 on mock2, got %d", mock2.tickCount)
	}
}

func TestMultiObserver_NTPSample(t *testing.T) {
	mock1 := &mockObserver{}
	mock2 := &mockObserver{}
	multi := NewMultiObserver(mock1, mock2)

	multi.NTPSample(time.Now(), 0.0, ptime.Time(0))

	if mock1.ntpSampleCount != 1 {
		t.Errorf("Expected NTPSample count 1 on mock1, got %d", mock1.ntpSampleCount)
	}
	if mock2.ntpSampleCount != 1 {
		t.Errorf("Expected NTPSample count 1 on mock2, got %d", mock2.ntpSampleCount)
	}
}