package nmeasyn

import (
	"math"
	"strings"
	"testing"
	"time"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/geopos"
	"github.com/jclark/satpulse/gps/lib/nmeamsg"
	"github.com/jclark/satpulse/gps/lib/opt"
	"github.com/jclark/satpulse/gps/ptime"
)

type sink struct {
	ggas []nmeamsg.GGASentence
}

func (s *sink) GGASentence(gga nmeamsg.GGASentence) {
	s.ggas = append(s.ggas, gga)
}

func TestSynthGGAFromGeo(t *testing.T) {
	sink := &sink{}
	s := New(sink)
	tRead := time.Unix(1, 0)
	s.Time(&gpsprot.TimeMsg{
		UTCTime: opt.Make(ptime.UTC(2026, 6, 24, 12, 34, 56, 0)),
	}, tRead)
	s.PosGeo(&gpsprot.PosGeoMsg{
		LatLon: [2]gpsprot.Angle{
			gpsprot.DegreesFromFloat(13.731826167),
			gpsprot.DegreesFromFloat(100.644802333),
		},
	}, tRead)
	s.NavEpoch(&gpsprot.NavEpochMsg{
		FixLevel:  gpsprot.FixLevelCode,
		NumSVUsed: opt.Make(uint16(8)),
		DOP:       gpsprot.DOP{Hor: opt.Make(1.2)},
	}, tRead)
	gga := oneGGA(t, sink)
	if gga.TalkerID() != "GN" {
		t.Fatalf("TalkerID = %q, want GN", gga.TalkerID())
	}
	if uint8(gga.Fields.Quality) != ggaQualitySPS {
		t.Fatalf("quality = %d, want %d", gga.Fields.Quality, ggaQualitySPS)
	}
	if payload := ggaPayload(t, gga); payload != "GNGGA,123456.00,1343.90957,N,10038.68814,E,1,08,1.2,,,,,," {
		t.Fatalf("payload = %q", payload)
	}
}

func TestSynthGGANoFix(t *testing.T) {
	sink := &sink{}
	s := New(sink)
	s.NavEpoch(&gpsprot.NavEpochMsg{FixLevel: gpsprot.FixLevelNone}, time.Time{})
	if gga := oneGGA(t, sink); uint8(gga.Fields.Quality) != ggaQualityInvalid {
		t.Fatalf("quality = %d, want %d", gga.Fields.Quality, ggaQualityInvalid)
	}
}

func TestSynthGGAFromECEF(t *testing.T) {
	sink := &sink{}
	s := New(sink)
	llh := geopos.LLH{Lat: -33.8688, Lon: -151.2093, Height: 50}
	ecef := geopos.WGS84.LLHtoECEF(llh)
	s.PosECEF(&gpsprot.PosECEFMsg{
		Pos: gpsprot.Point3D{
			gpsprot.Meters(ecef[0]),
			gpsprot.Meters(ecef[1]),
			gpsprot.Meters(ecef[2]),
		},
	}, time.Time{})
	s.NavEpoch(&gpsprot.NavEpochMsg{FixLevel: gpsprot.FixLevelCarrierFixed}, time.Time{})
	lat, lon := oneGGA(t, sink).Fields.LatLon()
	if math.Abs(lat-llh.Lat) > 1e-5 || math.Abs(lon-llh.Lon) > 1e-5 {
		t.Fatalf("lat/lon = %v, %v; want %v, %v", lat, lon, llh.Lat, llh.Lon)
	}
}

func TestSynthGGAPrefersGeoOverECEF(t *testing.T) {
	sink := &sink{}
	s := New(sink)
	ecef := geopos.WGS84.LLHtoECEF(geopos.LLH{Lat: -33.8688, Lon: -151.2093, Height: 50})
	s.PosECEF(&gpsprot.PosECEFMsg{
		Pos: gpsprot.Point3D{
			gpsprot.Meters(ecef[0]),
			gpsprot.Meters(ecef[1]),
			gpsprot.Meters(ecef[2]),
		},
	}, time.Time{})
	s.PosGeo(&gpsprot.PosGeoMsg{
		LatLon: [2]gpsprot.Angle{
			gpsprot.DegreesFromFloat(0),
			gpsprot.DegreesFromFloat(0),
		},
	}, time.Time{})
	s.NavEpoch(&gpsprot.NavEpochMsg{FixLevel: gpsprot.FixLevelCarrierFixed}, time.Time{})
	lat, lon := oneGGA(t, sink).Fields.LatLon()
	if lat != 0 || lon != 0 {
		t.Fatalf("lat/lon = %v, %v; want 0, 0", lat, lon)
	}
}

func TestGGAQuality(t *testing.T) {
	tests := []struct {
		name string
		msg  gpsprot.NavEpochMsg
		want uint8
	}{
		{"none", gpsprot.NavEpochMsg{FixLevel: gpsprot.FixLevelNone}, ggaQualityInvalid},
		{"dead reckoning", gpsprot.NavEpochMsg{FixLevel: gpsprot.FixLevelNone, AuxSrc: gpsprot.AuxSrcDR}, ggaQualityEstimated},
		{"code", gpsprot.NavEpochMsg{FixLevel: gpsprot.FixLevelCode}, ggaQualitySPS},
		{"dgps", gpsprot.NavEpochMsg{FixLevel: gpsprot.FixLevelCode, Correction: gpsprot.CorrUsed}, ggaQualityDGPS},
		{"float", gpsprot.NavEpochMsg{FixLevel: gpsprot.FixLevelCarrierFloat}, ggaQualityRTKFloat},
		{"fixed", gpsprot.NavEpochMsg{FixLevel: gpsprot.FixLevelCarrierFixed}, ggaQualityRTKFixed},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := ggaQuality(&tc.msg); got != tc.want {
				t.Fatalf("ggaQuality = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestGGANumSats(t *testing.T) {
	tests := []struct {
		in   uint16
		want uint8
	}{
		{0, 0},
		{8, 8},
		{nmeamsg.MaxGGANumSats, nmeamsg.MaxGGANumSats},
		{nmeamsg.MaxGGANumSats + 1, nmeamsg.MaxGGANumSats},
	}
	for _, tc := range tests {
		if got := ggaNumSats(tc.in); got != tc.want {
			t.Fatalf("ggaNumSats(%d) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

func oneGGA(t *testing.T, sink *sink) nmeamsg.GGASentence {
	t.Helper()
	if len(sink.ggas) != 1 {
		t.Fatalf("got %d GGA messages, want 1", len(sink.ggas))
	}
	return sink.ggas[0]
}

func ggaPayload(t *testing.T, gga nmeamsg.GGASentence) string {
	t.Helper()
	b, err := nmeamsg.SerializeMsg(gga)
	if err != nil {
		t.Fatalf("SerializeMsg: %v", err)
	}
	s := string(b)
	s = strings.TrimPrefix(s, "$")
	return s[:strings.IndexByte(s, '*')]
}
