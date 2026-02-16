package quectel

import (
	"math"
	"testing"
	"time"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/nmeamsg"
	"github.com/jclark/satpulse/gps/ptime"
)

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
	var epoch gpsprot.NavEpochMsg
	b, eoe := h.HandleSentence(propFlags, pvtPayload, &epoch)
	if eoe {
		t.Fatal("unexpected eoe")
	}
	if b == nil {
		t.Fatal("expected non-nil bundle")
	}
	// TimeMsg
	if b.Time == nil {
		t.Fatal("expected TimeMsg")
	}
	if b.Time.UTCTime == nil {
		t.Fatal("expected UTCTime")
	}
	wantUTC := ptime.UTC(2022, 12, 25, 8, 37, 37, 0)
	if *b.Time.UTCTime != wantUTC {
		t.Errorf("UTCTime = %v, want %v", *b.Time.UTCTime, wantUTC)
	}
	if b.Time.UTCOffset != 37 {
		t.Errorf("UTCOffset = %d, want 37", b.Time.UTCOffset)
	}
	if !b.Time.TAITime.IsZero() {
		t.Error("expected zero TAITime for PVT")
	}
	if b.Time.NativeMsgID != "PQTMPVT" {
		t.Errorf("NativeMsgID = %q, want PQTMPVT", b.Time.NativeMsgID)
	}
	// PosGeoMsg
	if b.PosGeo == nil {
		t.Fatal("expected PosGeoMsg")
	}
	if b.PosGeo.LatLon[0] != gpsprot.DegreesFromFloat(31.12738291) {
		t.Errorf("Lat = %v, want %v", b.PosGeo.LatLon[0], gpsprot.DegreesFromFloat(31.12738291))
	}
	if b.PosGeo.LatLon[1] != gpsprot.DegreesFromFloat(117.26372910) {
		t.Errorf("Lon = %v, want %v", b.PosGeo.LatLon[1], gpsprot.DegreesFromFloat(117.26372910))
	}
	if !b.PosGeo.HeightMSL.IsSet() || b.PosGeo.HeightMSL.Get() != gpsprot.Meters(34.212) {
		t.Errorf("HeightMSL = %v, want %v", b.PosGeo.HeightMSL.Get(), gpsprot.Meters(34.212))
	}
	if !b.PosGeo.Height.IsSet() || b.PosGeo.Height.Get() != gpsprot.Meters(34.212+5.267) {
		t.Errorf("Height = %v, want %v", b.PosGeo.Height.Get(), gpsprot.Meters(34.212+5.267))
	}
	// VelGeoMsg
	if b.VelGeo == nil {
		t.Fatal("expected VelGeoMsg")
	}
	if !b.VelGeo.VelNED.IsSet() {
		t.Fatal("expected VelNED")
	}
	ned := b.VelGeo.VelNED.Get()
	if ned[0] != gpsprot.MetersPerSecondFromFloat(3.212) {
		t.Errorf("VelN = %v, want %v", ned[0], gpsprot.MetersPerSecondFromFloat(3.212))
	}
	if ned[1] != gpsprot.MetersPerSecondFromFloat(2.928) {
		t.Errorf("VelE = %v, want %v", ned[1], gpsprot.MetersPerSecondFromFloat(2.928))
	}
	if ned[2] != gpsprot.MetersPerSecondFromFloat(0.238) {
		t.Errorf("VelD = %v, want %v", ned[2], gpsprot.MetersPerSecondFromFloat(0.238))
	}
	if !b.VelGeo.Speed3D.IsSet() || b.VelGeo.Speed3D.Get() != gpsprot.MetersPerSecondFromFloat(4.346) {
		t.Errorf("Speed3D = %v, want %v", b.VelGeo.Speed3D.Get(), gpsprot.MetersPerSecondFromFloat(4.346))
	}
	if !b.VelGeo.Course.IsSet() || b.VelGeo.Course.Get() != gpsprot.DegreesFromFloat(34.12) {
		t.Errorf("Course = %v, want %v", b.VelGeo.Course.Get(), gpsprot.DegreesFromFloat(34.12))
	}
	if b.VelGeo.GroundSpeed.IsSet() {
		t.Error("PVT should not set GroundSpeed")
	}
}

func TestPVTNoFix(t *testing.T) {
	payload := "PQTMPVT,1,31075000,20221225,083737.000,,0,00,,,,,,,,,,,,"
	h := NewHandler()
	var epoch gpsprot.NavEpochMsg
	b, eoe := h.HandleSentence(propFlags, payload, &epoch)
	if eoe {
		t.Fatal("unexpected eoe")
	}
	if b == nil {
		t.Fatal("expected non-nil bundle")
	}
	if b.Time != nil {
		t.Error("expected no TimeMsg for FixType 0")
	}
	if b.PosGeo != nil {
		t.Error("expected no PosGeoMsg without lat/lon")
	}
	if b.VelGeo != nil {
		t.Error("expected no VelGeoMsg without velocity")
	}
}

func TestNAVBundle(t *testing.T) {
	h := NewHandler()
	var epoch gpsprot.NavEpochMsg
	b, eoe := h.HandleSentence(propFlags, navPayload, &epoch)
	if eoe {
		t.Fatal("unexpected eoe")
	}
	if b == nil {
		t.Fatal("expected non-nil bundle")
	}
	// TimeMsg
	if b.Time == nil {
		t.Fatal("expected TimeMsg")
	}
	wantUTC := ptime.UTC(2024, 12, 24, 19, 4, 23, 0)
	if *b.Time.UTCTime != wantUTC {
		t.Errorf("UTCTime = %v, want %v", *b.Time.UTCTime, wantUTC)
	}
	wantTAI := ptime.GPS(2346, time.Duration(212681000)*time.Millisecond)
	if b.Time.TAITime != wantTAI {
		t.Errorf("TAITime = %v, want %v", b.Time.TAITime, wantTAI)
	}
	if b.Time.UTCOffset != 37 {
		t.Errorf("UTCOffset = %d, want 37", b.Time.UTCOffset)
	}
	if b.Time.NativeMsgID != "PQTMNAV" {
		t.Errorf("NativeMsgID = %q, want PQTMNAV", b.Time.NativeMsgID)
	}
	// PosGeoMsg
	if b.PosGeo == nil {
		t.Fatal("expected PosGeoMsg")
	}
	if b.PosGeo.LatLon[0] != gpsprot.DegreesFromFloat(31.45874521) {
		t.Errorf("Lat = %v, want %v", b.PosGeo.LatLon[0], gpsprot.DegreesFromFloat(31.45874521))
	}
	if !b.PosGeo.HeightMSL.IsSet() || b.PosGeo.HeightMSL.Get() != gpsprot.Meters(45.1254) {
		t.Errorf("HeightMSL = %v, want %v", b.PosGeo.HeightMSL.Get(), gpsprot.Meters(45.1254))
	}
	if !b.PosGeo.Height.IsSet() || b.PosGeo.Height.Get() != gpsprot.Meters(45.1254+(-6.1245)) {
		t.Errorf("Height = %v, want %v", b.PosGeo.Height.Get(), gpsprot.Meters(45.1254-6.1245))
	}
	// VelGeoMsg (NAV provides GroundSpeed from HVel, no VelNED)
	if b.VelGeo == nil {
		t.Fatal("expected VelGeoMsg")
	}
	if !b.VelGeo.GroundSpeed.IsSet() || b.VelGeo.GroundSpeed.Get() != gpsprot.MetersPerSecondFromFloat(1.2101) {
		t.Errorf("GroundSpeed = %v, want %v", b.VelGeo.GroundSpeed.Get(), gpsprot.MetersPerSecondFromFloat(1.2101))
	}
	if !b.VelGeo.Course.IsSet() || b.VelGeo.Course.Get() != gpsprot.DegreesFromFloat(45.124) {
		t.Errorf("Course = %v, want %v", b.VelGeo.Course.Get(), gpsprot.DegreesFromFloat(45.124))
	}
	if b.VelGeo.VelNED.IsSet() {
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
}

func TestNAVTimeInvalid(t *testing.T) {
	// TimeStatus=0 -> no TimeMsg
	payload := "PQTMNAV,1,0,1,190423.000,20241224,212681000,2346,18,,,12,,31.45874521,117.41532415,45.1254,-6.1245,,,1.2451,2.1254,5.1242,,,290,1.0,,78,56,,,,,,,1.2101,1.2148,0.4578,1.1547,,,45.124,,"
	h := NewHandler()
	var epoch gpsprot.NavEpochMsg
	b, _ := h.HandleSentence(propFlags, payload, &epoch)
	if b == nil {
		t.Fatal("expected non-nil bundle")
	}
	if b.Time != nil {
		t.Error("expected no TimeMsg when TimeStatus=0")
	}
	if b.PosGeo == nil {
		t.Error("expected PosGeoMsg even without valid time")
	}
}

func TestVELBundle(t *testing.T) {
	h := NewHandler()
	var epoch gpsprot.NavEpochMsg
	b, eoe := h.HandleSentence(propFlags, velPayload, &epoch)
	if eoe {
		t.Fatal("unexpected eoe")
	}
	if b == nil {
		t.Fatal("expected non-nil bundle")
	}
	if b.VelGeo == nil {
		t.Fatal("expected VelGeoMsg")
	}
	if !b.VelGeo.VelNED.IsSet() {
		t.Fatal("expected VelNED")
	}
	ned := b.VelGeo.VelNED.Get()
	if ned[0] != gpsprot.MetersPerSecondFromFloat(1.251) {
		t.Errorf("VelN = %v, want %v", ned[0], gpsprot.MetersPerSecondFromFloat(1.251))
	}
	if !b.VelGeo.GroundSpeed.IsSet() || b.VelGeo.GroundSpeed.Get() != gpsprot.MetersPerSecondFromFloat(2.752) {
		t.Errorf("GroundSpeed = %v, want %v", b.VelGeo.GroundSpeed.Get(), gpsprot.MetersPerSecondFromFloat(2.752))
	}
	if !b.VelGeo.Speed3D.IsSet() || b.VelGeo.Speed3D.Get() != gpsprot.MetersPerSecondFromFloat(3.021) {
		t.Errorf("Speed3D = %v, want %v", b.VelGeo.Speed3D.Get(), gpsprot.MetersPerSecondFromFloat(3.021))
	}
	if !b.VelGeo.Course.IsSet() || b.VelGeo.Course.Get() != gpsprot.DegreesFromFloat(180.512) {
		t.Errorf("Course = %v, want %v", b.VelGeo.Course.Get(), gpsprot.DegreesFromFloat(180.512))
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
	if b.Time != nil {
		t.Error("VEL should not produce TimeMsg")
	}
}

func TestEPEAccuracy(t *testing.T) {
	h := NewHandler()
	var epoch gpsprot.NavEpochMsg
	b, eoe := h.HandleSentence(propFlags, epePayload, &epoch)
	if eoe {
		t.Fatal("unexpected eoe")
	}
	if b == nil {
		t.Fatal("expected non-nil bundle")
	}
	// EPE produces no messages in the bundle
	if b.Time != nil || b.PosGeo != nil || b.VelGeo != nil || b.Survey != nil {
		t.Error("EPE should produce empty bundle")
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
	var epoch gpsprot.NavEpochMsg
	b, eoe := h.HandleSentence(propFlags, svinPayload, &epoch)
	if eoe {
		t.Fatal("unexpected eoe")
	}
	if b == nil {
		t.Fatal("expected non-nil bundle")
	}
	if b.Survey == nil {
		t.Fatal("expected SurveyMsg")
	}
	sv := b.Survey
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
	var epoch gpsprot.NavEpochMsg
	b, eoe := h.HandleSentence(propFlags, eoePayload, &epoch)
	if !eoe {
		t.Fatal("expected eoe=true for EOE")
	}
	if b == nil {
		t.Fatal("expected non-nil bundle")
	}
}

func TestNonProprietaryFlags(t *testing.T) {
	h := NewHandler()
	var epoch gpsprot.NavEpochMsg
	b, eoe := h.HandleSentence(0, pvtPayload, &epoch)
	if b != nil || eoe {
		t.Error("expected nil for non-proprietary flags")
	}
}

func TestNonPQTMPayload(t *testing.T) {
	h := NewHandler()
	var epoch gpsprot.NavEpochMsg
	b, eoe := h.HandleSentence(propFlags, "GPGGA,1,2,3", &epoch)
	if b != nil || eoe {
		t.Error("expected nil for non-PQTM payload")
	}
}

func TestConfigResponse(t *testing.T) {
	// PQTMCFGMSGRATE is not a periodic message; should return nil
	h := NewHandler()
	var epoch gpsprot.NavEpochMsg
	b, eoe := h.HandleSentence(propFlags, "PQTMCFGMSGRATE,OK", &epoch)
	if b != nil || eoe {
		t.Error("expected nil for config response")
	}
}

func TestUnrecognizedPQTM(t *testing.T) {
	h := NewHandler()
	var epoch gpsprot.NavEpochMsg
	b, eoe := h.HandleSentence(propFlags, "PQTMFOO,1,2,3", &epoch)
	if b != nil || eoe {
		t.Error("expected nil for unrecognized PQTM message")
	}
}

func TestDOPNotConverted(t *testing.T) {
	// DOP is a recognized periodic message but has no gpsprot mapping yet
	h := NewHandler()
	var epoch gpsprot.NavEpochMsg
	b, eoe := h.HandleSentence(propFlags, "PQTMDOP,1,570643000,1.01,0.88,0.49,0.73,0.50,0.36,0.35", &epoch)
	if b != nil || eoe {
		t.Error("expected nil for DOP (no gpsprot mapping)")
	}
}
