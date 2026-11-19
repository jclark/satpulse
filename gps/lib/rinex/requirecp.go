package rinex

import "github.com/jclark/satpulse/gps/lib/opt"

// RequireCPFilter wraps a Sink and drops observations that have no carrier
// phase. A pseudorange-only signal is a valid observation, but PPP-AR
// processors such as CSRS-PPP treat such rows as tracking interruptions, so
// filtering them produces output better suited to PPP-AR.
type RequireCPFilter struct {
	sink Sink
}

// NewRequireCPFilter creates a Sink that forwards only observations with a
// carrier phase to sink.
func NewRequireCPFilter(sink Sink) *RequireCPFilter {
	return &RequireCPFilter{sink: sink}
}

// Metadata passes m through to the wrapped sink.
func (s *RequireCPFilter) Metadata(m Metadata) error {
	return s.sink.Metadata(m)
}

// Observation emits obs only when it carries a carrier phase.
func (s *RequireCPFilter) Observation(obs SignalObservation) error {
	if !obs.CP.IsSet() {
		return nil
	}
	return s.sink.Observation(obs)
}

// Flush flushes the wrapped sink.
func (s *RequireCPFilter) Flush() error {
	return s.sink.Flush()
}

// OmitDoWithoutCPFilter wraps a Sink and omits the Doppler of observations
// that have no carrier phase, dropping observations left with no values.
// This matches RTKLIB Explorer's Unicore and NovAtel decoders, which zero
// Doppler along with carrier phase when the phase lock flag is clear.
type OmitDoWithoutCPFilter struct {
	sink Sink
}

// NewOmitDoWithoutCPFilter creates a Sink that forwards observations to sink
// without the Doppler of those that have no carrier phase.
func NewOmitDoWithoutCPFilter(sink Sink) *OmitDoWithoutCPFilter {
	return &OmitDoWithoutCPFilter{sink: sink}
}

// Metadata passes m through to the wrapped sink.
func (s *OmitDoWithoutCPFilter) Metadata(m Metadata) error {
	return s.sink.Metadata(m)
}

// Observation emits obs, without its Doppler if it has no carrier phase.
func (s *OmitDoWithoutCPFilter) Observation(obs SignalObservation) error {
	if !obs.CP.IsSet() {
		obs.Do = opt.Val[float64]{}
		if len(obs.ObservationCodes()) == 0 {
			return nil
		}
	}
	return s.sink.Observation(obs)
}

// Flush flushes the wrapped sink.
func (s *OmitDoWithoutCPFilter) Flush() error {
	return s.sink.Flush()
}
