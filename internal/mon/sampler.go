package mon

import (
	"iter"
	"time"

	"github.com/jclark/satpulse/internal/ptime"
)

// SampleKind determines the validity and type of a synchronization sample
type SampleKind int

const (
	SampleMissing SampleKind = iota // Missing sample (PPS signal not received)
	SampleOK                        // Valid sample within acceptable limits
	SampleOutlier                   // Sample that is an outlier but still measured
)

// SyncState represents the current synchronization status
type SyncState int

const (
	NoSync SyncState = iota // Clock is not synchronized
	InSync                  // Clock is synchronized
)

// String returns a human-readable representation of the sync state
func (s SyncState) String() string {
	switch s {
	case NoSync:
		return "out of sync"
	case InSync:
		return "in sync"
	default:
		return "unknown"
	}
}

// SampleData contains all information about a synchronization sample
type SampleData struct {
	Kind      SampleKind    // Determines validity of other fields
	Ref       ptime.Time    // GPS reference time (different from system time)
	Offset    time.Duration // PHC/GPS offset (valid for SampleOK and SampleOutlier, 0 for SampleMissing)
	Freq      float64       // Current frequency adjustment in PPB (always valid)
	SyncState SyncState     // Current synchronization state (always valid)
	Era       ptime.Era     // For clock step tracking and logging (always valid)
}

// Sampler handles clock synchronization samples of all types
type Sampler interface {
	// Sample reports a clock synchronization sample of any kind
	Sample(data SampleData)
}

// MultiSampler fans out Sample calls to multiple samplers
type MultiSampler struct {
	samplers []Sampler
}

// NewMultiSampler creates a new MultiSampler that fans out to multiple samplers
func NewMultiSampler(samplers ...Sampler) *MultiSampler {
	return &MultiSampler{samplers: samplers}
}

// Sample implements Sampler by calling Sample on all samplers
func (m *MultiSampler) Sample(data SampleData) {
	for _, s := range m.samplers {
		s.Sample(data)
	}
}

// Samplers returns an iterator over the samplers
func (m *MultiSampler) Samplers() iter.Seq[Sampler] {
	return func(yield func(Sampler) bool) {
		for _, s := range m.samplers {
			if !yield(s) {
				return
			}
		}
	}
}