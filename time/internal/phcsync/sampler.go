package phcsync

import (
	"iter"
	"time"

	"github.com/jclark/satpulse/time/phctime"
	"github.com/jclark/satpulse/gps/ptime"
)

// SampleKind determines the validity and type of a synchronization sample
type SampleKind int

const (
	SampleMissing SampleKind = iota // Missing sample (PPS signal not received)
	SampleOK                        // Valid sample within acceptable limits
	SampleOutlier                   // Sample that is an outlier but still measured
)

// Sample contains all information about a synchronization sample
type Sample struct {
	Kind      SampleKind    // Determines validity of other fields
	Ref       ptime.Time    // GPS reference time (different from system time)
	Offset    time.Duration // PHC/GPS offset (valid for SampleOK and SampleOutlier, 0 for SampleMissing)
	Freq      float64       // Current frequency adjustment in PPB (always valid)
	FreqDelta float64       // Change in frequency adjustment in PPB (valid for SampleOK, 0 for SampleOutlier)
	Mode      Mode          // Current synchronization mode (always valid)
	Era       phctime.Era   // For clock step tracking and logging (always valid)
	EdgeIndex uint64        // Tracks which edge produced this sample (odd/even)
	Sys       time.Time     // Estimated monotonic system time of pulse
}

// SysSample returns a phctime.Sample pairing the GPS reference time with
// the estimated system time of the pulse.
func (s *Sample) SysSample() phctime.Sample {
	return phctime.Sample{
		PHC: phctime.Time{T: s.Ref, Era: s.Era},
		Sys: s.Sys,
	}
}

// Observer receives clock synchronization samples and mode-change events
// from the controller.
type Observer interface {
	// Sample reports a clock synchronization sample of any kind.
	// data.Mode is the mode the sample was served in; if the sample
	// triggered a transition, ModeChanged will be called immediately
	// after with the new mode.
	Sample(data Sample)

	// ModeChanged reports a transition in the controller's sync mode.
	// It fires from every transition path, including those not driven
	// by a sample (e.g. carrier-loss Pause). At controller startup the
	// initial transition out of ModeInvalid into ModeReset is also
	// reported here.
	ModeChanged(oldMode, newMode Mode)
}

// MultiObserver fans out events to multiple observers.
type MultiObserver struct {
	observers []Observer
}

// NewMultiObserver creates a new MultiObserver that fans out to multiple observers.
func NewMultiObserver(observers ...Observer) *MultiObserver {
	return &MultiObserver{observers: observers}
}

// Sample implements Observer by calling Sample on all observers.
func (m *MultiObserver) Sample(data Sample) {
	for _, o := range m.observers {
		o.Sample(data)
	}
}

// ModeChanged implements Observer by calling ModeChanged on all observers.
func (m *MultiObserver) ModeChanged(oldMode, newMode Mode) {
	for _, o := range m.observers {
		o.ModeChanged(oldMode, newMode)
	}
}

// Observers returns an iterator over the observers.
func (m *MultiObserver) Observers() iter.Seq[Observer] {
	return func(yield func(Observer) bool) {
		for _, o := range m.observers {
			if !yield(o) {
				return
			}
		}
	}
}
