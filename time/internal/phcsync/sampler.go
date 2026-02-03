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

// Sampler handles clock synchronization samples of all types
type Sampler interface {
	// Sample reports a clock synchronization sample of any kind
	Sample(data Sample)
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
func (m *MultiSampler) Sample(data Sample) {
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
