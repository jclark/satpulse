package combine

import "github.com/jclark/satpulse/internal/ptime"

type multiSampler struct {
	samplers []Sampler
}

func (ms *multiSampler) Sample(ref ptime.Time, local ptime.ClockTime, delayed bool) {
	for _, sampler := range ms.samplers {
		sampler.Sample(ref, local, delayed)
	}
}

func MultiSampler(samplers ...Sampler) Sampler {
	return &multiSampler{samplers: samplers}
}
