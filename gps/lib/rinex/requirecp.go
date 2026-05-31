package rinex

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
