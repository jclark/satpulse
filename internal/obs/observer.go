package obs

import (
	"github.com/jclark/satpulse/internal/gpsprot"
	"github.com/jclark/satpulse/internal/mon"
)

// Observer provides unified observability interface
type Observer interface {
	mon.Sampler
	gpsprot.MsgHandler
	
	// Release releases any resources used by the observer
	Release()
}

// MultiObserver fans out calls to multiple observers
type MultiObserver struct {
	*gpsprot.MultiHandler
}

// NewMultiObserver creates a new MultiObserver that fans out to multiple observers
func NewMultiObserver(observers ...Observer) *MultiObserver {
	handlers := make([]gpsprot.MsgHandler, len(observers))
	for i, obs := range observers {
		handlers[i] = obs
	}
	
	return &MultiObserver{
		MultiHandler: gpsprot.NewMultiHandler(handlers...),
	}
}

// Sample implements mon.Sampler by type-asserting handlers to Sampler
func (m *MultiObserver) Sample(data mon.SampleData) {
	for h := range m.Handlers() {
		if sampler, ok := h.(mon.Sampler); ok {
			sampler.Sample(data)
		}
	}
}

// Release implements Observer by type-asserting handlers to Observer
func (m *MultiObserver) Release() {
	for h := range m.Handlers() {
		if obs, ok := h.(Observer); ok {
			obs.Release()
		}
	}
}

// DefaultObserver is a no-op implementation of Observer
type DefaultObserver struct {
	gpsprot.DefaultHandler
}

// Sample implements mon.Sampler as a no-op
func (o *DefaultObserver) Sample(data mon.SampleData) {}

// Release implements Observer as a no-op
func (o *DefaultObserver) Release() {}