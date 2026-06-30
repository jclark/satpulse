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
	ggas   []nmeamsg.GGASentence
	phases []Phase
}

func (s *sink) Msg(m nmeamsg.GNSSTalkerIDMsg, phase Phase) {
	if gga, ok := m.(nmeamsg.GGASentence); ok {
		s.ggas = append(s.ggas, gga)
		s.phases = append(s.phases, phase)
	}
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

func TestSynthGGAFromGeoWithoutTimeUsesEpochInstant(t *testing.T) {
	sink := &sink{}
	s := New(sink)
	// With no UTC, the GGA time is the end-of-epoch read time minus the
	// first-message-to-end delay, i.e. the first message's read time.
	first := time.Date(2026, 6, 24, 9, 8, 7, 50000000, time.UTC)
	s.PosGeo(&gpsprot.PosGeoMsg{
		LatLon: [2]gpsprot.Angle{
			gpsprot.DegreesFromFloat(13.731826167),
			gpsprot.DegreesFromFloat(100.644802333),
		},
	}, first)
	s.NavEpoch(&gpsprot.NavEpochMsg{
		FixLevel: gpsprot.FixLevelCode,
	}, time.Date(2026, 6, 24, 9, 8, 8, 0, time.UTC))
	if payload := ggaPayload(t, oneGGA(t, sink)); !strings.HasPrefix(payload, "GNGGA,090807.05,1343.90957,N,10038.68814,E,1,") {
		t.Fatalf("payload = %q, want epoch-instant time", payload)
	}
}

func TestSynthGGAEpochDelayFromFirstMessage(t *testing.T) {
	sink := &sink{}
	s := New(sink)
	// The delay is measured from the first message (the no-UTC Time here), not a
	// later one, so the reconstructed time is the first message's read time.
	s.Time(&gpsprot.TimeMsg{}, time.Date(2026, 6, 24, 9, 8, 7, 0, time.UTC))
	s.PosGeo(&gpsprot.PosGeoMsg{
		LatLon: [2]gpsprot.Angle{
			gpsprot.DegreesFromFloat(13.731826167),
			gpsprot.DegreesFromFloat(100.644802333),
		},
	}, time.Date(2026, 6, 24, 9, 8, 8, 0, time.UTC))
	s.NavEpoch(&gpsprot.NavEpochMsg{FixLevel: gpsprot.FixLevelCode}, time.Date(2026, 6, 24, 9, 8, 9, 0, time.UTC))
	if payload := ggaPayload(t, oneGGA(t, sink)); !strings.HasPrefix(payload, "GNGGA,090807.00,") {
		t.Fatalf("payload = %q, want first (Time) message read time", payload)
	}
}

func TestSynthGGARemembersEpochDelay(t *testing.T) {
	sink := &sink{}
	s := New(sink)
	pos := &gpsprot.PosGeoMsg{
		LatLon: [2]gpsprot.Angle{
			gpsprot.DegreesFromFloat(13.731826167),
			gpsprot.DegreesFromFloat(100.644802333),
		},
	}
	// Epoch 1 has a first message, fixing the 0.9s delay.
	s.PosGeo(pos, time.Date(2026, 6, 24, 10, 0, 0, 0, time.UTC))
	s.NavEpoch(&gpsprot.NavEpochMsg{FixLevel: gpsprot.FixLevelCode}, time.Date(2026, 6, 24, 10, 0, 0, 900000000, time.UTC))
	// Epoch 2 has no message before NavEpoch, so the remembered 0.9s delay is
	// subtracted from its end-of-epoch read time.
	s.NavEpoch(&gpsprot.NavEpochMsg{FixLevel: gpsprot.FixLevelNone}, time.Date(2026, 6, 24, 10, 0, 1, 900000000, time.UTC))
	if len(sink.ggas) != 2 {
		t.Fatalf("got %d GGA messages, want 2", len(sink.ggas))
	}
	if got := ggaPayload(t, sink.ggas[0]); !strings.HasPrefix(got, "GNGGA,100000.00,") {
		t.Fatalf("epoch 1 payload = %q, want 100000.00", got)
	}
	if got := ggaPayload(t, sink.ggas[1]); !strings.HasPrefix(got, "GNGGA,100001.00,,,,,0,") {
		t.Fatalf("epoch 2 payload = %q, want 100001.00 no-fix", got)
	}
}

func TestSynthGGAFromGeoHeights(t *testing.T) {
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
		Height:    opt.Make(gpsprot.Meters(100)),
		HeightMSL: opt.Make(gpsprot.Meters(75.5)),
	}, tRead)
	s.NavEpoch(&gpsprot.NavEpochMsg{
		FixLevel:  gpsprot.FixLevelCode,
		NumSVUsed: opt.Make(uint16(8)),
		DOP:       gpsprot.DOP{Hor: opt.Make(1.2)},
	}, tRead)
	if payload := ggaPayload(t, oneGGA(t, sink)); payload != "GNGGA,123456.00,1343.90957,N,10038.68814,E,1,08,1.2,75.5,M,24.5,M,," {
		t.Fatalf("payload = %q", payload)
	}
}

func TestSynthGGAFromGeoHeightWithoutMSL(t *testing.T) {
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
		Height: opt.Make(gpsprot.Meters(100)),
	}, tRead)
	s.NavEpoch(&gpsprot.NavEpochMsg{
		FixLevel:  gpsprot.FixLevelCode,
		NumSVUsed: opt.Make(uint16(8)),
		DOP:       gpsprot.DOP{Hor: opt.Make(1.2)},
	}, tRead)
	if payload := ggaPayload(t, oneGGA(t, sink)); payload != "GNGGA,123456.00,1343.90957,N,10038.68814,E,1,08,1.2,100,M,0,M,," {
		t.Fatalf("payload = %q", payload)
	}
}

func TestSynthGGANoFix(t *testing.T) {
	sink := &sink{}
	s := New(sink)
	// No prior message, so the delay is zero and the GGA time is the NavEpoch
	// read time.
	s.NavEpoch(&gpsprot.NavEpochMsg{FixLevel: gpsprot.FixLevelNone}, time.Date(2026, 6, 24, 1, 2, 3, 0, time.UTC))
	gga := oneGGA(t, sink)
	if uint8(gga.Fields.Quality) != ggaQualityInvalid {
		t.Fatalf("quality = %d, want %d", gga.Fields.Quality, ggaQualityInvalid)
	}
	if gga.Fields.NumSats.IsSet() || gga.Fields.HDOP.IsSet() {
		t.Fatalf("optional metadata = %v/%v, want unset", gga.Fields.NumSats, gga.Fields.HDOP)
	}
	if payload := ggaPayload(t, gga); payload != "GNGGA,010203.00,,,,,0,,,,,,,," {
		t.Fatalf("payload = %q", payload)
	}
}

func TestSynthGGAFixWithoutPosition(t *testing.T) {
	sink := &sink{}
	s := New(sink)
	// A fix level with no PosGeo/PosECEF in the epoch must not yield a
	// quality > 0 GGA at 0,0; report no fix and leave the position empty.
	s.NavEpoch(&gpsprot.NavEpochMsg{FixLevel: gpsprot.FixLevelCarrierFixed}, time.Time{})
	gga := oneGGA(t, sink)
	if uint8(gga.Fields.Quality) != ggaQualityInvalid {
		t.Fatalf("quality = %d, want %d", gga.Fields.Quality, ggaQualityInvalid)
	}
	if gga.Fields.Lat.IsSet() || gga.Fields.Lon.IsSet() {
		t.Fatalf("position set without a position message")
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
	if sink.phases[0] != PhaseEpoch {
		t.Fatalf("phase = %d, want PhaseEpoch", sink.phases[0])
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
