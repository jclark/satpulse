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

// Sink receives synthesized NMEA GGA sentences.
type Sink interface {
	GGASentence(gga nmeamsg.GGASentence)
}

// Synth accumulates protocol-neutral messages for one navigation epoch and
// emits a synthesized GGA when the epoch ends.
type Synth struct {
	gpsprot.DefaultHandler
	sink   Sink
	time   time.Time
	latLon *[2]float64
	ecef   *geopos.ECEF
}

// New creates a GGA synthesizer.
func New(sink Sink) *Synth {
	return &Synth{sink: sink}
}

// Time records the UTC time for the current epoch.
func (s *Synth) Time(msg *gpsprot.TimeMsg, _ time.Time) {
	if msg.Ref == gpsprot.PrePulse || !msg.UTCTime.IsSet() {
		return
	}
	s.time = msg.UTCTime.Get().SysTime()
}

// PosGeo records the geodetic position for the current epoch.
func (s *Synth) PosGeo(msg *gpsprot.PosGeoMsg, _ time.Time) {
	ll := [2]float64{msg.LatLon[0].Degrees(), msg.LatLon[1].Degrees()}
	s.latLon = &ll
}

// PosECEF records the ECEF position for the current epoch.
func (s *Synth) PosECEF(msg *gpsprot.PosECEFMsg, _ time.Time) {
	ecef := geopos.ECEF{msg.Pos[0].Meters(), msg.Pos[1].Meters(), msg.Pos[2].Meters()}
	s.ecef = &ecef
}

// NavEpoch emits synthesized GGA for the epoch and clears accumulated state.
func (s *Synth) NavEpoch(msg *gpsprot.NavEpochMsg, _ time.Time) {
	latLon := s.position()
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
	s.sink.GGASentence(nmeamsg.MakeGGA("GN", s.time, latLon, quality, numSats, hdop))
	s.clear()
}

func (s *Synth) position() *[2]float64 {
	if s.latLon != nil {
		return s.latLon
	}
	if s.ecef == nil {
		return nil
	}
	llh, err := geopos.WGS84.ECEFtoLLH(*s.ecef)
	if err != nil {
		return nil
	}
	return &[2]float64{llh.Lat, llh.Lon}
}

func (s *Synth) clear() {
	s.time = time.Time{}
	s.latLon = nil
	s.ecef = nil
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
