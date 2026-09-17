package casbin

import (
	"reflect"
	"strings"
	"testing"
)

func TestRxm2MeasxParse(t *testing.T) {
	// Synthetic one-measurement packet exercising every documented field.
	pkt := parseHex(t, "bace3000130044332211feffffff6655ee011f030000000000000000f83f00000000000002c00000604002070509cdab2d0607005308cd3e036b")
	msg, err := ParseMsg(pkt)
	if err != nil {
		t.Fatalf("ParseMsg error: %v", err)
	}
	measx, ok := msg.(*Rxm2Measx)
	if !ok {
		t.Fatalf("expected *Rxm2Measx, got %T", msg)
	}
	if measx.RawTOW != 0x11223344 || measx.RawTOWSubms != -2 || measx.Wn != 0x5566 {
		t.Errorf("epoch time = (%#x, %d, %#x), want (0x11223344, -2, 0x5566)", measx.RawTOW, measx.RawTOWSubms, measx.Wn)
	}
	if measx.LeapSec != -18 || measx.NumMeas != 1 {
		t.Errorf("LeapSec/NumMeas = %d/%d, want -18/1", measx.LeapSec, measx.NumMeas)
	}
	if measx.TimeFlags != Rxm2TimeTOWValid|Rxm2TimeWNValid|Rxm2TimeLeapValid|Rxm2TimeReliable|Rxm2TimeClockJump {
		t.Errorf("TimeFlags = 0x%02x, want 0x1f", measx.TimeFlags)
	}
	if measx.TSrc != Tim2TSrcGAL {
		t.Errorf("TSrc = %d, want GAL", measx.TSrc)
	}
	if len(measx.Meas) != 1 {
		t.Fatalf("len(Meas) = %d, want 1", len(measx.Meas))
	}
	m := measx.Meas[0]
	if m.PRMes != 1.5 || m.CPMes != -2.25 || m.CPRateMes != 3.5 {
		t.Errorf("measurements = (%g, %g, %g), want (1.5, -2.25, 3.5)", m.PRMes, m.CPMes, m.CPRateMes)
	}
	if m.GNSSID != GLN || m.SVID != 7 || m.SigID != SigGLOL1 || m.FreqID != 9 {
		t.Errorf("signal IDs = (%d, %d, %d, %d), want (GLN, 7, GLOL1, 9)", m.GNSSID, m.SVID, m.SigID, m.FreqID)
	}
	if m.CPLockTime != 0xabcd || m.CNO != 45 || m.PRRMS != 6 || m.DORMS != 7 {
		t.Errorf("signal quality = (%#x, %d, %d, %d), want (0xabcd, 45, 6, 7)", m.CPLockTime, m.CNO, m.PRRMS, m.DORMS)
	}
	if m.TrkFlags != Rxm2TrackingPRValid|Rxm2TrackingCPValid|Rxm2TrackingNoMSAmbiguity|Rxm2TrackingElevationLow {
		t.Errorf("TrkFlags = 0x%02x, want 0x53", m.TrkFlags)
	}
	if m.Chn != 8 {
		t.Errorf("Chn = %d, want 8", m.Chn)
	}
}

func TestRxm2MeasxRoundtrip(t *testing.T) {
	m := Rxm2Measx{
		Rxm2MeasxFixed: Rxm2MeasxFixed{
			RawTOW:      123456789,
			RawTOWSubms: -12345,
			Wn:          2422,
			LeapSec:     18,
			NumMeas:     2,
			TimeFlags:   Rxm2TimeTOWValid | Rxm2TimeWNValid,
			TSrc:        Tim2TSrcGPS,
		},
		Meas: []Rxm2MeasxMeas{
			{
				PRMes:      20888187.25,
				CPMes:      20888195.5,
				CPRateMes:  -181.5,
				GNSSID:     GPS,
				SVID:       5,
				SigID:      SigGPSL1CA,
				CPLockTime: 65535,
				CNO:        51,
				PRRMS:      2,
				DORMS:      3,
				TrkFlags:   Rxm2TrackingPRValid | Rxm2TrackingCPValid,
				Chn:        4,
			},
			{
				PRMes:      24123456.75,
				CPMes:      24123460.125,
				CPRateMes:  222.75,
				GNSSID:     GLN,
				SVID:       8,
				SigID:      SigGLOL1,
				FreqID:     9,
				CPLockTime: 12000,
				CNO:        43,
				PRRMS:      4,
				DORMS:      5,
				TrkFlags:   Rxm2TrackingPRValid,
				Chn:        12,
			},
		},
	}
	got := testMsgType1[Rxm2Measx, *Rxm2Measx](t, m).(*Rxm2Measx)
	if !reflect.DeepEqual(got, &m) {
		t.Fatalf("roundtrip changed message:\ngot  %+v\nwant %+v", got, &m)
	}
}

func TestRxm2MeasxBadLength(t *testing.T) {
	pkt, err := PackMsg(Rxm2MeasxID, make([]byte, 17))
	if err != nil {
		t.Fatal(err)
	}
	_, err = ParseMsg(string(pkt))
	if err == nil || !strings.Contains(err.Error(), "payload length") {
		t.Fatalf("ParseMsg error = %v, want bad payload length", err)
	}
}

func TestRxm2MeasxBadCount(t *testing.T) {
	payload := make([]byte, 16+32)
	payload[11] = 2
	pkt, err := PackMsg(Rxm2MeasxID, payload)
	if err != nil {
		t.Fatal(err)
	}
	_, err = ParseMsg(string(pkt))
	if err == nil || !strings.Contains(err.Error(), "measurement count") {
		t.Fatalf("ParseMsg error = %v, want bad measurement count", err)
	}
}
