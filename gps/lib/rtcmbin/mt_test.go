package rtcmbin

import (
	"encoding/hex"
	"testing"
)

func TestParseMT1005(t *testing.T) {
	result, err := ParseMsg(rtcm1005)
	if err != nil {
		t.Fatal(err)
	}
	msg, ok := result.(*MT1005)
	if !ok {
		t.Fatalf("ParseMsg returned %T, want *MT1005", result)
	}
	if msg.MsgNum != 1005 {
		t.Errorf("MsgNum = %d, want 1005", msg.MsgNum)
	}
	if msg.MsgType() != 1005 {
		t.Errorf("MsgType() = %d, want 1005", msg.MsgType())
	}
	// Cross-check with ReferenceStationID extraction
	wantID, _ := ReferenceStationID([]byte(rtcm1005))
	if msg.StationID != wantID {
		t.Errorf("StationID = %d, want %d", msg.StationID, wantID)
	}
	if !msg.GPS {
		t.Error("GPS = false, want true")
	}
	ecef := msg.ECEF()
	t.Logf("MT1005: station=%d ITRF=%d GPS=%v GLO=%v GAL=%v ref=%v",
		msg.StationID, msg.ITRFYear, msg.GPS, msg.GLONASS, msg.Galileo, msg.RefStation)
	t.Logf("ECEF: X=%.4f Y=%.4f Z=%.4f m", ecef[0], ecef[1], ecef[2])
}

func TestParseMT1005Real(t *testing.T) {
	pkt, err := hex.DecodeString("d300133ed4d203bd55b51d7f8e2e1f5ad403808e27daf56b52")
	if err != nil {
		t.Fatal(err)
	}
	result, err := ParseMsg(string(pkt))
	if err != nil {
		t.Fatal(err)
	}
	msg, ok := result.(*MT1005)
	if !ok {
		t.Fatalf("ParseMsg returned %T, want *MT1005", result)
	}
	if msg.StationID != 1234 {
		t.Errorf("StationID = %d, want 1234", msg.StationID)
	}
	// Cross-check with ReferenceStationID extraction
	wantID, idOK := ReferenceStationID(pkt)
	if !idOK {
		t.Fatal("ReferenceStationID returned false")
	}
	if msg.StationID != wantID {
		t.Errorf("StationID = %d, want %d (from ReferenceStationID)", msg.StationID, wantID)
	}
	ecef := msg.ECEF()
	t.Logf("MT1005: station=%d ITRF=%d GPS=%v GLO=%v GAL=%v ref=%v singleOsc=%v qc=%d",
		msg.StationID, msg.ITRFYear, msg.GPS, msg.GLONASS, msg.Galileo, msg.RefStation,
		msg.SingleOsc, msg.QuarterCycle)
	t.Logf("ECEF: X=%.4f Y=%.4f Z=%.4f m", ecef[0], ecef[1], ecef[2])
}

func TestParseMsgUnknown(t *testing.T) {
	pkt := "\xD3\x00\x01\x00\x00\x00\x00\x00"
	result, err := ParseMsg(pkt)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := result.(*UnknownMsg); !ok {
		t.Errorf("ParseMsg returned %T, want *UnknownMsg", result)
	}
}
