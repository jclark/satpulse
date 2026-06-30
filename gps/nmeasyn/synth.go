// Package nmeasyn synthesizes NMEA sentences from protocol-neutral GPS data.
package nmeasyn

import (
	"time"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/geopos"
	"github.com/jclark/satpulse/gps/lib/nmeamsg"
)

const (
	ggaQualityInvalid uint8 = iota
	ggaQualitySPS
	ggaQualityDGPS
	ggaQualityPPS
	ggaQualityRTKFixed
	ggaQualityRTKFloat
	ggaQualityEstimated
	ggaQualityManualInput
	ggaQualitySimulator
)

// Phase is the timeliness contract of a synthesized sentence. PhaseEpoch carries
// an epoch's full metadata; PhaseImmediate (not yet emitted) favours timeliness
// over fidelity. A sink applies its own policy for which phase to consume.
type Phase int

const (
	PhaseImmediate Phase = iota
	PhaseEpoch
)

// Sink receives synthesized NMEA sentences. The sentence is the general
// nmeamsg.GNSSTalkerIDMsg; a sink recovers the concrete type (e.g.
// nmeamsg.GGASentence) with a type assertion.
type Sink interface {
	Msg(m nmeamsg.GNSSTalkerIDMsg, phase Phase)
}

// Synth accumulates protocol-neutral messages for one navigation epoch and
// emits a synthesized GGA when the epoch ends.
type Synth struct {
	gpsprot.DefaultHandler
	sink       Sink
	time       time.Time
	tRead      time.Time
	epochDelay time.Duration
	latLon     *[2]float64
	height     *float64
	heightMSL  *float64
	ecef       *geopos.ECEF
}

// New creates a GGA synthesizer.
func New(sink Sink) *Synth {
	return &Synth{sink: sink}
}

// Time records the UTC time for the current epoch.
func (s *Synth) Time(msg *gpsprot.TimeMsg, tRead time.Time) {
	s.noteTRead(tRead)
	if msg.Ref == gpsprot.PrePulse || !msg.UTCTime.IsSet() {
		return
	}
	s.time = msg.UTCTime.Get().SysTime()
}

// PosGeo records the geodetic position for the current epoch.
func (s *Synth) PosGeo(msg *gpsprot.PosGeoMsg, tRead time.Time) {
	s.noteTRead(tRead)
	ll := [2]float64{msg.LatLon[0].Degrees(), msg.LatLon[1].Degrees()}
	s.latLon = &ll
	s.height = lengthMeters(msg.Height.Ptr())
	s.heightMSL = lengthMeters(msg.HeightMSL.Ptr())
}

// PosECEF records the ECEF position for the current epoch.
func (s *Synth) PosECEF(msg *gpsprot.PosECEFMsg, tRead time.Time) {
	s.noteTRead(tRead)
	ecef := geopos.ECEF{msg.Pos[0].Meters(), msg.Pos[1].Meters(), msg.Pos[2].Meters()}
	s.ecef = &ecef
}

// noteTRead records the read time of the first message of the epoch, used at
// NavEpoch to measure the epoch's first-message-to-end delay.
func (s *Synth) noteTRead(tRead time.Time) {
	if s.tRead.IsZero() {
		s.tRead = tRead
	}
}

// NavEpoch emits synthesized GGA for the epoch and clears accumulated state.
func (s *Synth) NavEpoch(msg *gpsprot.NavEpochMsg, tRead time.Time) {
	if !s.tRead.IsZero() {
		s.epochDelay = tRead.Sub(s.tRead)
	}
	latLon, height := s.position()
	quality := ggaQuality(msg)
	if latLon == nil {
		quality = ggaQualityInvalid // no position, so report no fix
	}
	var numSats *uint8
	if n := msg.NumSVUsed.Ptr(); n != nil {
		v := ggaNumSats(*n)
		numSats = &v
	}
	var hdop *float64
	if v := msg.DOP.Hor.Ptr(); v != nil {
		hdop = v
	}
	heightMSL := s.heightMSL
	if height != nil && heightMSL == nil {
		heightMSL = height
	}
	t := s.time
	if t.IsZero() {
		// No UTC this epoch: reconstruct the epoch instant from the end-of-epoch
		// read time minus the learned first-message-to-end delay, so the time
		// stays consistent with UTC-stamped epochs and monotonic even if the
		// system clock is offset.  A zero delay falls through to tRead.
		t = tRead.Add(-s.epochDelay)
	}
	s.sink.Msg(nmeamsg.MakeGGA("GN", t, latLon, height, heightMSL, quality, numSats, hdop), PhaseEpoch)
	s.clear()
}

func (s *Synth) position() (*[2]float64, *float64) {
	if s.latLon != nil {
		return s.latLon, s.height
	}
	if s.ecef == nil {
		return nil, nil
	}
	llh, err := geopos.WGS84.ECEFtoLLH(*s.ecef)
	if err != nil {
		return nil, nil
	}
	return &[2]float64{llh.Lat, llh.Lon}, &llh.Height
}

func (s *Synth) clear() {
	s.time = time.Time{}
	s.tRead = time.Time{}
	s.latLon = nil
	s.height = nil
	s.heightMSL = nil
	s.ecef = nil
}

func lengthMeters(l *gpsprot.Length) *float64 {
	if l == nil {
		return nil
	}
	v := l.Meters()
	return &v
}

func ggaNumSats(n uint16) uint8 {
	if n > nmeamsg.MaxGGANumSats {
		return nmeamsg.MaxGGANumSats
	}
	return uint8(n)
}

func ggaQuality(msg *gpsprot.NavEpochMsg) uint8 {
	if msg.FixLevel < gpsprot.FixLevelCode {
		if msg.AuxSrc&gpsprot.AuxSrcDR != 0 {
			return ggaQualityEstimated
		}
		return ggaQualityInvalid
	}
	switch msg.FixLevel {
	case gpsprot.FixLevelCode:
		if msg.Correction&gpsprot.CorrUsed != 0 {
			return ggaQualityDGPS
		}
		return ggaQualitySPS
	case gpsprot.FixLevelCarrierFloat:
		return ggaQualityRTKFloat
	case gpsprot.FixLevelCarrierFixed:
		return ggaQualityRTKFixed
	}
	return ggaQualityInvalid
}
