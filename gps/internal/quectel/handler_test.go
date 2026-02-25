package quectel

import (
	"math"
	"testing"
	"time"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/nmeamsg"
	"github.com/jclark/satpulse/gps/ptime"
)

// msgRec dispatches []gpsprot.Msg and captures individual message types.
type msgRec struct {
	gpsprot.DefaultHandler
	times   []*gpsprot.TimeMsg
	posGeo  []*gpsprot.PosGeoMsg
	velGeo  []*gpsprot.VelGeoMsg
	surveys []*gpsprot.SurveyMsg
}

func (r *msgRec) Time(m *gpsprot.TimeMsg, _ time.Time)       { r.times = append(r.times, m) }
func (r *msgRec) PosGeo(m *gpsprot.PosGeoMsg, _ time.Time)   { r.posGeo = append(r.posGeo, m) }
func (r *msgRec) VelGeo(m *gpsprot.VelGeoMsg, _ time.Time)   { r.velGeo = append(r.velGeo, m) }
func (r *msgRec) Survey(m *gpsprot.SurveyMsg, _ time.Time)   { r.surveys = append(r.surveys, m) }

func dispatch(msgs []gpsprot.Msg) *msgRec {
	var r msgRec
	gpsprot.DispatchMsgs(msgs, &r, time.Time{})
	return &r
}

const propFlags = nmeamsg.SentenceProprietaryNMEA

// Payloads reuse the examples from qtmmsg/periodic_test.go.
const (
	pvtPayload  = "PQTMPVT,1,31075000,20221225,083737.000,,3,09,18,31.12738291,117.26372910,34.212,5.267,3.212,2.928,0.238,4.346,34.12,2.16,4.38"
	velPayload  = "PQTMVEL,1,154512.100,1.251,2.452,1.245,2.752,3.021,180.512,0.124,0.254,0.250"
	epePayload  = "PQTMEPE,2,1.000,1.000,1.000,1.414,1.732"
	svinPayload = "PQTMSVINSTATUS,1,1000,1,01,20,100,-2484434.3645,4875976.9741,3266161.3412,1.2415"
	navPayload  = "PQTMNAV,1,1,1,190423.000,20241224,212681000,2346,18,,,12,,31.45874521,117.41532415,45.1254,-6.1245,,,1.2451,2.1254,5.1242,,,290,1.0,,78,56,,,,,,,1.2101,1.2148,0.4578,1.1547,,,45.124,,"
	eoePayload  = "PQTMEOE,1,190423.000,20241224,2346,212681000"
)

func TestPVTBundle(t *testing.T) {
	h := NewHandler()
	msgs, epoch, err := h.HandleSentence(propFlags, pvtPayload, nil)
	if err != nil {
		t.Fatal(err)
	}
	r := dispatch(msgs)
	// TimeMsg
	if len(r.times) != 1 {
		t.Fatal("expected 1 TimeMsg")
	}
	tm := r.times[0]
	if tm.UTCTime == nil {
		t.Fatal("expected UTCTime")
	}
	wantUTC := ptime.UTC(2022, 12, 25, 8, 37, 37, 0)
	if *tm.UTCTime != wantUTC {
		t.Errorf("UTCTime = %v, want %v", *tm.UTCTime, wantUTC)
	}
	if tm.UTCOffset != 37 {
		t.Errorf("UTCOffset = %d, want 37", tm.UTCOffset)
	}
	if !tm.TAITime.IsZero() {
		t.Error("expected zero TAITime for PVT")
	}
	if tm.NativeMsgID != "PQTMPVT" {
		t.Errorf("NativeMsgID = %q, want PQTMPVT", tm.NativeMsgID)
	}
	// PosGeoMsg
	if len(r.posGeo) != 1 {
		t.Fatal("expected 1 PosGeoMsg")
	}
	pg := r.posGeo[0]
	if pg.LatLon[0] != gpsprot.DegreesFromFloat(31.12738291) {
		t.Errorf("Lat = %v, want %v", pg.LatLon[0], gpsprot.DegreesFromFloat(31.12738291))
	}
	if pg.LatLon[1] != gpsprot.DegreesFromFloat(117.26372910) {
		t.Errorf("Lon = %v, want %v", pg.LatLon[1], gpsprot.DegreesFromFloat(117.26372910))
	}
	if !pg.HeightMSL.IsSet() || pg.HeightMSL.Get() != gpsprot.Meters(34.212) {
		t.Errorf("HeightMSL = %v, want %v", pg.HeightMSL.Get(), gpsprot.Meters(34.212))
	}
	if !pg.Height.IsSet() || pg.Height.Get() != gpsprot.Meters(34.212+5.267) {
		t.Errorf("Height = %v, want %v", pg.Height.Get(), gpsprot.Meters(34.212+5.267))
	}
	// VelGeoMsg
	if len(r.velGeo) != 1 {
		t.Fatal("expected 1 VelGeoMsg")
	}
	vg := r.velGeo[0]
	if !vg.VelNED.IsSet() {
		t.Fatal("expected VelNED")
	}
	ned := vg.VelNED.Get()
	if ned[0] != gpsprot.MetersPerSecondFromFloat(3.212) {
		t.Errorf("VelN = %v, want %v", ned[0], gpsprot.MetersPerSecondFromFloat(3.212))
	}
	if ned[1] != gpsprot.MetersPerSecondFromFloat(2.928) {
		t.Errorf("VelE = %v, want %v", ned[1], gpsprot.MetersPerSecondFromFloat(2.928))
	}
	if ned[2] != gpsprot.MetersPerSecondFromFloat(0.238) {
		t.Errorf("VelD = %v, want %v", ned[2], gpsprot.MetersPerSecondFromFloat(0.238))
	}
	if !vg.Speed3D.IsSet() || vg.Speed3D.Get() != gpsprot.MetersPerSecondFromFloat(4.346) {
		t.Errorf("Speed3D = %v, want %v", vg.Speed3D.Get(), gpsprot.MetersPerSecondFromFloat(4.346))
	}
	if !vg.Course.IsSet() || vg.Course.Get() != gpsprot.DegreesFromFloat(34.12) {
		t.Errorf("Course = %v, want %v", vg.Course.Get(), gpsprot.DegreesFromFloat(34.12))
	}
	if vg.GroundSpeed.IsSet() {
		t.Error("PVT should not set GroundSpeed")
	}
	// Quality fields from PVT (FixType=3)
	if epoch.FixLevel != gpsprot.FixLevelCode {
		t.Errorf("FixLevel = %d, want FixLevelCode", epoch.FixLevel)
	}
	if epoch.FixDim != gpsprot.FixDim3D {
		t.Errorf("FixDim = %d, want FixDim3D", epoch.FixDim)
	}
	if !epoch.NumSVUsed.IsSet() || epoch.NumSVUsed.Get() != 9 {
		t.Errorf("NumSVUsed = %v, want 9", epoch.NumSVUsed)
	}
	if !epoch.DOP.Hor.IsSet() || epoch.DOP.Hor.Get() != 2.16 {
		t.Errorf("DOP.Hor = %v, want 2.16", epoch.DOP.Hor)
	}
	if !epoch.DOP.Pos.IsSet() || epoch.DOP.Pos.Get() != 4.38 {
		t.Errorf("DOP.Pos = %v, want 4.38", epoch.DOP.Pos)
	}
}

func TestPVTNoFix(t *testing.T) {
	payload := "PQTMPVT,1,31075000,20221225,083737.000,,0,00,,,,,,,,,,,,"
	h := NewHandler()
	msgs, epoch, err := h.HandleSentence(propFlags, payload, nil)
	if err != nil {
		t.Fatal(err)
	}
	r := dispatch(msgs)
	if len(r.times) != 0 {
		t.Error("expected no TimeMsg for FixType 0")
	}
	if len(r.posGeo) != 0 {
		t.Error("expected no PosGeoMsg without lat/lon")
	}
	if len(r.velGeo) != 0 {
		t.Error("expected no VelGeoMsg without velocity")
	}
	if epoch.FixLevel != gpsprot.FixLevelNone {
		t.Errorf("FixLevel = %d, want FixLevelNone", epoch.FixLevel)
	}
	if epoch.FixDim != 0 {
		t.Errorf("FixDim = %d, want 0", epoch.FixDim)
	}
}

func TestNAVBundle(t *testing.T) {
	h := NewHandler()
	msgs, epoch, err := h.HandleSentence(propFlags, navPayload, nil)
	if err != nil {
		t.Fatal(err)
	}
	r := dispatch(msgs)
	// TimeMsg
	if len(r.times) != 1 {
		t.Fatal("expected 1 TimeMsg")
	}
	tm := r.times[0]
	wantUTC := ptime.UTC(2024, 12, 24, 19, 4, 23, 0)
	if *tm.UTCTime != wantUTC {
		t.Errorf("UTCTime = %v, want %v", *tm.UTCTime, wantUTC)
	}
	wantTAI := ptime.GPS(2346, time.Duration(212681000)*time.Millisecond)
	if tm.TAITime != wantTAI {
		t.Errorf("TAITime = %v, want %v", tm.TAITime, wantTAI)
	}
	if tm.UTCOffset != 37 {
		t.Errorf("UTCOffset = %d, want 37", tm.UTCOffset)
	}
	if tm.NativeMsgID != "PQTMNAV" {
		t.Errorf("NativeMsgID = %q, want PQTMNAV", tm.NativeMsgID)
	}
	// PosGeoMsg
	if len(r.posGeo) != 1 {
		t.Fatal("expected 1 PosGeoMsg")
	}
	pg := r.posGeo[0]
	if pg.LatLon[0] != gpsprot.DegreesFromFloat(31.45874521) {
		t.Errorf("Lat = %v, want %v", pg.LatLon[0], gpsprot.DegreesFromFloat(31.45874521))
	}
	if !pg.HeightMSL.IsSet() || pg.HeightMSL.Get() != gpsprot.Meters(45.1254) {
		t.Errorf("HeightMSL = %v, want %v", pg.HeightMSL.Get(), gpsprot.Meters(45.1254))
	}
	if !pg.Height.IsSet() || pg.Height.Get() != gpsprot.Meters(45.1254+(-6.1245)) {
		t.Errorf("Height = %v, want %v", pg.Height.Get(), gpsprot.Meters(45.1254-6.1245))
	}
	// VelGeoMsg (NAV provides GroundSpeed from HVel, no VelNED)
	if len(r.velGeo) != 1 {
		t.Fatal("expected 1 VelGeoMsg")
	}
	vg := r.velGeo[0]
	if !vg.GroundSpeed.IsSet() || vg.GroundSpeed.Get() != gpsprot.MetersPerSecondFromFloat(1.2101) {
		t.Errorf("GroundSpeed = %v, want %v", vg.GroundSpeed.Get(), gpsprot.MetersPerSecondFromFloat(1.2101))
	}
	if !vg.Course.IsSet() || vg.Course.Get() != gpsprot.DegreesFromFloat(45.124) {
		t.Errorf("Course = %v, want %v", vg.Course.Get(), gpsprot.DegreesFromFloat(45.124))
	}
	if vg.VelNED.IsSet() {
		t.Error("NAV should not set VelNED")
	}
	// NavEpochMsg accuracy from NAV
	wantHor := gpsprot.Meters(math.Sqrt(1.2451*1.2451 + 2.1254*2.1254))
	if !epoch.Acc.Hor.IsSet() || epoch.Acc.Hor.Get() != wantHor {
		t.Errorf("Acc.Hor = %v, want %v", epoch.Acc.Hor.Get(), wantHor)
	}
	if !epoch.Acc.Vert.IsSet() || epoch.Acc.Vert.Get() != gpsprot.Meters(5.1242) {
		t.Errorf("Acc.Vert = %v, want %v", epoch.Acc.Vert.Get(), gpsprot.Meters(5.1242))
	}
	if !epoch.Acc.GroundSpeed.IsSet() || epoch.Acc.GroundSpeed.Get() != gpsprot.MetersPerSecondFromFloat(0.4578) {
		t.Errorf("Acc.GroundSpeed = %v, want %v", epoch.Acc.GroundSpeed.Get(), gpsprot.MetersPerSecondFromFloat(0.4578))
	}
	// Quality fields from NAV (SolType=12 -> RTK fixed)
	if epoch.FixLevel != gpsprot.FixLevelCarrierFixed {
		t.Errorf("FixLevel = %d, want FixLevelCarrierFixed", epoch.FixLevel)
	}
	if epoch.FixDim != gpsprot.FixDim3D {
		t.Errorf("FixDim = %d, want FixDim3D", epoch.FixDim)
	}
	wantCorr := gpsprot.CorrBaseStation | gpsprot.CorrUsed
	if epoch.Correction != wantCorr {
		t.Errorf("Correction = %d, want %d", epoch.Correction, wantCorr)
	}
	if !epoch.NumSVUsed.IsSet() || epoch.NumSVUsed.Get() != 56 {
		t.Errorf("NumSVUsed = %v, want 56", epoch.NumSVUsed)
	}
	if !epoch.NumSVTracked.IsSet() || epoch.NumSVTracked.Get() != 78 {
		t.Errorf("NumSVTracked = %v, want 78", epoch.NumSVTracked)
	}
	if !epoch.DiffAge.IsSet() || epoch.DiffAge.Get() != gpsprot.Second {
		t.Errorf("DiffAge = %v, want 1s", epoch.DiffAge)
	}
	if !epoch.RTCMRefBaseID.IsSet() || epoch.RTCMRefBaseID.Get() != 290 {
		t.Errorf("RTCMRefBaseID = %v, want 290", epoch.RTCMRefBaseID)
	}
}

func TestNAVTimeInvalid(t *testing.T) {
	// TimeStatus=0 -> no TimeMsg
	payload := "PQTMNAV,1,0,1,190423.000,20241224,212681000,2346,18,,,12,,31.45874521,117.41532415,45.1254,-6.1245,,,1.2451,2.1254,5.1242,,,290,1.0,,78,56,,,,,,,1.2101,1.2148,0.4578,1.1547,,,45.124,,"
	h := NewHandler()
	msgs, _, err := h.HandleSentence(propFlags, payload, nil)
	if err != nil {
		t.Fatal(err)
	}
	r := dispatch(msgs)
	if len(r.times) != 0 {
		t.Error("expected no TimeMsg when TimeStatus=0")
	}
	if len(r.posGeo) == 0 {
		t.Error("expected PosGeoMsg even without valid time")
	}
}

func TestVELBundle(t *testing.T) {
	h := NewHandler()
	msgs, epoch, err := h.HandleSentence(propFlags, velPayload, nil)
	if err != nil {
		t.Fatal(err)
	}
	r := dispatch(msgs)
	if len(r.velGeo) != 1 {
		t.Fatal("expected 1 VelGeoMsg")
	}
	vg := r.velGeo[0]
	if !vg.VelNED.IsSet() {
		t.Fatal("expected VelNED")
	}
	ned := vg.VelNED.Get()
	if ned[0] != gpsprot.MetersPerSecondFromFloat(1.251) {
		t.Errorf("VelN = %v, want %v", ned[0], gpsprot.MetersPerSecondFromFloat(1.251))
	}
	if !vg.GroundSpeed.IsSet() || vg.GroundSpeed.Get() != gpsprot.MetersPerSecondFromFloat(2.752) {
		t.Errorf("GroundSpeed = %v, want %v", vg.GroundSpeed.Get(), gpsprot.MetersPerSecondFromFloat(2.752))
	}
	if !vg.Speed3D.IsSet() || vg.Speed3D.Get() != gpsprot.MetersPerSecondFromFloat(3.021) {
		t.Errorf("Speed3D = %v, want %v", vg.Speed3D.Get(), gpsprot.MetersPerSecondFromFloat(3.021))
	}
	if !vg.Course.IsSet() || vg.Course.Get() != gpsprot.DegreesFromFloat(180.512) {
		t.Errorf("Course = %v, want %v", vg.Course.Get(), gpsprot.DegreesFromFloat(180.512))
	}
	// Epoch accuracy
	if !epoch.Acc.GroundSpeed.IsSet() || epoch.Acc.GroundSpeed.Get() != gpsprot.MetersPerSecondFromFloat(0.124) {
		t.Errorf("Acc.GroundSpeed = %v, want %v", epoch.Acc.GroundSpeed.Get(), gpsprot.MetersPerSecondFromFloat(0.124))
	}
	if !epoch.Acc.Speed.IsSet() || epoch.Acc.Speed.Get() != gpsprot.MetersPerSecondFromFloat(0.254) {
		t.Errorf("Acc.Speed = %v, want %v", epoch.Acc.Speed.Get(), gpsprot.MetersPerSecondFromFloat(0.254))
	}
	if !epoch.Acc.Course.IsSet() || epoch.Acc.Course.Get() != gpsprot.DegreesFromFloat(0.250) {
		t.Errorf("Acc.Course = %v, want %v", epoch.Acc.Course.Get(), gpsprot.DegreesFromFloat(0.250))
	}
	if len(r.times) != 0 {
		t.Error("VEL should not produce TimeMsg")
	}
}

func TestEPEAccuracy(t *testing.T) {
	h := NewHandler()
	msgs, epoch, err := h.HandleSentence(propFlags, epePayload, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 0 {
		t.Error("EPE should produce no messages")
	}
	if !epoch.Acc.Hor.IsSet() || epoch.Acc.Hor.Get() != gpsprot.Meters(1.414) {
		t.Errorf("Acc.Hor = %v, want %v", epoch.Acc.Hor.Get(), gpsprot.Meters(1.414))
	}
	if !epoch.Acc.Pos.IsSet() || epoch.Acc.Pos.Get() != gpsprot.Meters(1.732) {
		t.Errorf("Acc.Pos = %v, want %v", epoch.Acc.Pos.Get(), gpsprot.Meters(1.732))
	}
	if !epoch.Acc.Vert.IsSet() || epoch.Acc.Vert.Get() != gpsprot.Meters(1.0) {
		t.Errorf("Acc.Vert = %v, want %v", epoch.Acc.Vert.Get(), gpsprot.Meters(1.0))
	}
}

func TestSVINStatusBundle(t *testing.T) {
	h := NewHandler()
	msgs, _, err := h.HandleSentence(propFlags, svinPayload, nil)
	if err != nil {
		t.Fatal(err)
	}
	r := dispatch(msgs)
	if len(r.surveys) != 1 {
		t.Fatal("expected 1 SurveyMsg")
	}
	sv := r.surveys[0]
	if !sv.InProgress {
		t.Error("expected InProgress for Valid=1")
	}
	if sv.Valid {
		t.Error("expected not Valid for Valid=1")
	}
	if sv.ObsCount != 20 {
		t.Errorf("ObsCount = %d, want 20", sv.ObsCount)
	}
	if sv.Position[0] != gpsprot.Meters(-2484434.3645) {
		t.Errorf("MeanX = %v, want %v", sv.Position[0], gpsprot.Meters(-2484434.3645))
	}
	if sv.Position[1] != gpsprot.Meters(4875976.9741) {
		t.Errorf("MeanY = %v, want %v", sv.Position[1], gpsprot.Meters(4875976.9741))
	}
	if sv.Position[2] != gpsprot.Meters(3266161.3412) {
		t.Errorf("MeanZ = %v, want %v", sv.Position[2], gpsprot.Meters(3266161.3412))
	}
	if sv.Accuracy != gpsprot.Meters(1.2415) {
		t.Errorf("Accuracy = %v, want %v", sv.Accuracy, gpsprot.Meters(1.2415))
	}
}

func TestEOE(t *testing.T) {
	h := NewHandler()
	msgs, epoch, err := h.HandleSentence(propFlags, eoePayload, nil)
	if err != nil {
		t.Fatal(err)
	}
	if epoch != nil {
		t.Fatal("expected nil epoch for EOE")
	}
	if msgs != nil {
		t.Fatal("expected nil msgs for EOE")
	}
}

func TestNonProprietaryFlags(t *testing.T) {
	h := NewHandler()
	_, _, err := h.HandleSentence(0, pvtPayload, nil)
	if err != gpsprot.ErrNotHandled {
		t.Errorf("expected ErrNotHandled, got %v", err)
	}
}

func TestNonPQTMPayload(t *testing.T) {
	h := NewHandler()
	_, _, err := h.HandleSentence(propFlags, "GPGGA,1,2,3", nil)
	if err != gpsprot.ErrNotHandled {
		t.Errorf("expected ErrNotHandled, got %v", err)
	}
}

func TestConfigResponse(t *testing.T) {
	// PQTMCFGMSGRATE is not a periodic message; should return ErrNotHandled
	h := NewHandler()
	_, _, err := h.HandleSentence(propFlags, "PQTMCFGMSGRATE,OK", nil)
	if err != gpsprot.ErrNotHandled {
		t.Errorf("expected ErrNotHandled, got %v", err)
	}
}

func TestUnrecognizedPQTM(t *testing.T) {
	h := NewHandler()
	_, _, err := h.HandleSentence(propFlags, "PQTMFOO,1,2,3", nil)
	if err != gpsprot.ErrNotHandled {
		t.Errorf("expected ErrNotHandled, got %v", err)
	}
}

func TestDOP(t *testing.T) {
	h := NewHandler()
	msgs, epoch, err := h.HandleSentence(propFlags, "PQTMDOP,1,570643000,1.01,0.88,0.49,0.73,0.50,0.36,0.35", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 0 {
		t.Error("DOP should produce no messages")
	}
	if !epoch.DOP.Geom.IsSet() || epoch.DOP.Geom.Get() != 1.01 {
		t.Errorf("DOP.Geom = %v, want 1.01", epoch.DOP.Geom)
	}
	if !epoch.DOP.Pos.IsSet() || epoch.DOP.Pos.Get() != 0.88 {
		t.Errorf("DOP.Pos = %v, want 0.88", epoch.DOP.Pos)
	}
	if !epoch.DOP.Time.IsSet() || epoch.DOP.Time.Get() != 0.49 {
		t.Errorf("DOP.Time = %v, want 0.49", epoch.DOP.Time)
	}
	if !epoch.DOP.Vert.IsSet() || epoch.DOP.Vert.Get() != 0.73 {
		t.Errorf("DOP.Vert = %v, want 0.73", epoch.DOP.Vert)
	}
	if !epoch.DOP.Hor.IsSet() || epoch.DOP.Hor.Get() != 0.50 {
		t.Errorf("DOP.Hor = %v, want 0.50", epoch.DOP.Hor)
	}
}

func TestNavSolQuality(t *testing.T) {
	tests := []struct {
		solType uint8
		fl      gpsprot.FixLevel
		fd      gpsprot.FixDim
		corr    gpsprot.CorrKind
		ok      bool
	}{
		{0, gpsprot.FixLevelNone, 0, 0, true},
		{1, gpsprot.FixLevelCode, gpsprot.FixDim3D, 0, true},
		{2, gpsprot.FixLevelCodeCorrected, gpsprot.FixDim3D, gpsprot.CorrSBAS | gpsprot.CorrWideArea | gpsprot.CorrUsed, true},
		{5, gpsprot.FixLevelCodeCorrected, gpsprot.FixDim3D, gpsprot.CorrBaseStation | gpsprot.CorrUsed, true},
		{8, gpsprot.FixLevelCarrierFloat, gpsprot.FixDim3D, gpsprot.CorrBaseStation | gpsprot.CorrUsed, true},
		{12, gpsprot.FixLevelCarrierFixed, gpsprot.FixDim3D, gpsprot.CorrBaseStation | gpsprot.CorrUsed, true},
		{99, 0, 0, 0, false},
	}
	for _, tt := range tests {
		fl, fd, corr, ok := navSolQuality(tt.solType)
		if fl != tt.fl || fd != tt.fd || corr != tt.corr || ok != tt.ok {
			t.Errorf("navSolQuality(%d) = (%d, %d, %d, %v), want (%d, %d, %d, %v)",
				tt.solType, fl, fd, corr, ok, tt.fl, tt.fd, tt.corr, tt.ok)
		}
	}
}

func TestNAVThenPVTDoesNotDowngrade(t *testing.T) {
	h := NewHandler()
	// NAV with SolType=12 (RTK fixed) sets CarrierFixed.
	_, epoch, err := h.HandleSentence(propFlags, navPayload, nil)
	if err != nil {
		t.Fatal(err)
	}
	if epoch.FixLevel != gpsprot.FixLevelCarrierFixed {
		t.Fatalf("after NAV: FixLevel = %d, want CarrierFixed", epoch.FixLevel)
	}
	// PVT with FixType=3 (3D) must not downgrade to FixLevelCode.
	// Use same time-of-day as NAV (190423.000) so CheckEpoch keeps the same epoch.
	pvtSameTime := "PQTMPVT,1,31075000,20241224,190423.000,,3,09,18,31.12738291,117.26372910,34.212,5.267,3.212,2.928,0.238,4.346,34.12,2.16,4.38"
	_, epoch, err = h.HandleSentence(propFlags, pvtSameTime, epoch)
	if err != nil {
		t.Fatal(err)
	}
	if epoch.FixLevel != gpsprot.FixLevelCarrierFixed {
		t.Errorf("after PVT: FixLevel = %d, want CarrierFixed (not downgraded)", epoch.FixLevel)
	}
	if epoch.FixDim != gpsprot.FixDim3D {
		t.Errorf("after PVT: FixDim = %d, want FixDim3D", epoch.FixDim)
	}
	wantCorr := gpsprot.CorrBaseStation | gpsprot.CorrUsed
	if epoch.Correction != wantCorr {
		t.Errorf("after PVT: Correction = %d, want %d", epoch.Correction, wantCorr)
	}
}

func TestUnknownNAVSolTypePreservesQuality(t *testing.T) {
	h := NewHandler()
	// PVT sets FixLevelCode.
	_, epoch, err := h.HandleSentence(propFlags, pvtPayload, nil)
	if err != nil {
		t.Fatal(err)
	}
	if epoch.FixLevel != gpsprot.FixLevelCode {
		t.Fatalf("after PVT: FixLevel = %d, want FixLevelCode", epoch.FixLevel)
	}
	// NAV with unknown SolType (99) must not zero out quality.
	// Use same time-of-day as PVT (083737.000) so CheckEpoch keeps the same epoch.
	unknownNav := "PQTMNAV,1,1,1,083737.000,20221225,212681000,2346,18,,,99,,31.45874521,117.41532415,45.1254,-6.1245,,,1.2451,2.1254,5.1242,,,290,1.0,,78,56,,,,,,,1.2101,1.2148,0.4578,1.1547,,,45.124,,"
	_, epoch, err = h.HandleSentence(propFlags, unknownNav, epoch)
	if err != nil {
		t.Fatal(err)
	}
	if epoch.FixLevel != gpsprot.FixLevelCode {
		t.Errorf("after unknown NAV: FixLevel = %d, want FixLevelCode (preserved)", epoch.FixLevel)
	}
}
