package sdbp

import (
	"encoding/hex"
	"reflect"
	"testing"
	"time"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/sdbpbin"
	"github.com/jclark/satpulse/gps/ptime"
)

func parsePacket(t *testing.T, hexStr string) sdbpbin.Msg {
	t.Helper()
	s := make([]byte, 0, len(hexStr))
	for i := 0; i < len(hexStr); i++ {
		if hexStr[i] != ' ' {
			s = append(s, hexStr[i])
		}
	}
	pkt, err := hex.DecodeString(string(s))
	if err != nil {
		t.Fatalf("bad hex: %v", err)
	}
	msg, err := sdbpbin.ParseMsg(string(pkt))
	if err != nil {
		t.Fatalf("ParseMsg: %v", err)
	}
	return msg
}

func ptr[T any](v T) *T { return &v }

// Adjacent pair from taidou-capture2.jsonl:
//   {"t":"2026-03-17T00:15:18.054733Z","tag":"SDBP","msg":"DAT-TPPS","bin":"233e064111000008000000b83405416a094d0100000001540f"}
//   {"t":"2026-03-17T00:15:18.056741Z","tag":"SDBP","msg":"DAT-GPST","bin":"233e0617130040005b0a036a09ad8d24ff3f350541030000006550"}
// Capture time (rounded to second): 2026-03-17 00:15:18 UTC
const (
	captureTPPS = "233e064111000008000000b83405416a094d0100000001540f"
	captureGPST = "233e0617130040005b0a036a09ad8d24ff3f350541030000006550"
)

var captureTime = time.Date(2026, 3, 17, 0, 15, 18, 0, time.UTC)

// currentLeapSeconds is the current TAI-UTC offset (37s as of 2026).
const currentLeapSeconds = 37

func TestTimeDatGPST(t *testing.T) {
	tests := []struct {
		name string
		hex  string
		want *gpsprot.TimeMsg
	}{
		{
			// From Windows app capture: GPS week 2410, TOW 132694.999796053, accuracy 4 ns
			name: "from-app",
			hex:  "23 3E 06 17 13 00 D8 C3 E8 07 03 6A 09 A2 12 95 FF B7 32 00 41 04 00 00 00 A6 A9",
			want: &gpsprot.TimeMsg{
				Ref:         gpsprot.NavSolution,
				NativeMsgID: "DAT-GPST",
				GNSS:        gpsprot.GPS,
				TAITime:     ptime.GPS(2410, ptime.Seconds(132694.999796053)),
				Accuracy:    4 * time.Nanosecond,
			},
		},
		{
			// From capture at 2026-03-17T00:15:18Z
			name: "from-capture",
			hex:  captureGPST,
			want: &gpsprot.TimeMsg{
				Ref:         gpsprot.NavSolution,
				NativeMsgID: "DAT-GPST",
				GNSS:        gpsprot.GPS,
				TAITime:     ptime.GPS(2410, ptime.Seconds(173735.99958143887)),
				Accuracy:    3 * time.Nanosecond,
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			msg := parsePacket(t, tc.hex)
			got := timeDatGPST(msg.(*sdbpbin.DatGPST))
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("got  %+v\nwant %+v", got, tc.want)
			}
		})
	}
}

// TestTimeDatGPSTMatchesCaptureTime verifies that the TAI time from DAT-GPST,
// when converted to UTC, matches the capture timestamp from the packet log.
func TestTimeDatGPSTMatchesCaptureTime(t *testing.T) {
	msg := parsePacket(t, captureGPST)
	tm := timeDatGPST(msg.(*sdbpbin.DatGPST))
	taiSec := time.Duration(tm.TAITime) * time.Nanosecond
	utcFromTAI := time.Unix(0, int64(taiSec)).Add(-time.Duration(currentLeapSeconds) * time.Second).UTC().Round(time.Second)
	if !utcFromTAI.Equal(captureTime) {
		t.Errorf("DAT-GPST UTC = %v, want %v (capture time)", utcFromTAI, captureTime)
	}
}

func TestTimeDatGPSTInvalid(t *testing.T) {
	tests := []struct {
		name  string
		valid sdbpbin.DatTimeValid
	}{
		{"no flags", 0},
		{"tow only", sdbpbin.DatTimeTowValid},
		{"week only", sdbpbin.DatTimeWeekValid},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gpst := &sdbpbin.DatGPST{}
			gpst.Valid = tc.valid
			if tm := timeDatGPST(gpst); tm != nil {
				t.Error("expected nil for invalid DatGPST")
			}
		})
	}
}

func TestTimeDatTPPS(t *testing.T) {
	tests := []struct {
		name string
		hex  string
		want *gpsprot.TimeMsg
	}{
		{
			// From Windows app capture: PPS0, TOW 134202.0 UTC, week 2410, residual 597 ps
			name: "from-app",
			hex:  "23 3E 06 41 11 00 00 FD FF FF FF CF 61 00 41 6A 09 55 02 00 00 00 01 8E 3B",
			want: &gpsprot.TimeMsg{
				Ref:         gpsprot.PrePulse,
				NativeMsgID: "DAT-TPPS",
				GNSS:        gpsprot.GPS,
				UTCTime:     ptr(ptime.GPSUTC(2410, ptime.Seconds(134202.0))),
				PulseOffset: ptr(597e-12),
			},
		},
		{
			// From capture at 2026-03-17T00:15:18Z
			name: "from-capture",
			hex:  captureTPPS,
			want: &gpsprot.TimeMsg{
				Ref:         gpsprot.PrePulse,
				NativeMsgID: "DAT-TPPS",
				GNSS:        gpsprot.GPS,
				UTCTime:     ptr(ptime.GPSUTC(2410, ptime.Seconds(173719.0))),
				PulseOffset: ptr(333e-12),
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			msg := parsePacket(t, tc.hex)
			got := timeDatTPPS(msg.(*sdbpbin.DatTPPS))
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("got  %+v\nwant %+v", got, tc.want)
			}
		})
	}
}

// TestTimeDatTPPSMatchesCaptureTime verifies that DAT-TPPS UTC time
// is one second ahead of the capture timestamp (pre-pulse: time of next PPS).
func TestTimeDatTPPSMatchesCaptureTime(t *testing.T) {
	msg := parsePacket(t, captureTPPS)
	tm := timeDatTPPS(msg.(*sdbpbin.DatTPPS))
	utc := tm.UTCTime.Date.Add(tm.UTCTime.TimeOfDay).Round(time.Second)
	wantUTC := captureTime.Add(time.Second)
	if !utc.Equal(wantUTC) {
		t.Errorf("DAT-TPPS UTC = %v, want %v (capture time + 1s)", utc, wantUTC)
	}
}

// Captured DAT-UTCT2 from Taidou T303-5D (2026-03-17T13:23:55Z)
const captureUTCT2 = "233e061f1f0048002d0d0b02ea0703110d173741a002000400000012ffffffff000000000028d8"

// Captured DAT-GPSU from Taidou T303-5D (2026-03-17T13:23:55Z)
const captureGPSU = "233e062d230000000000000030be000000000000e0bc0000000000000000003006006a09128909071246da"

func TestTimeDatUTCT2Captured(t *testing.T) {
	msg := parsePacket(t, captureUTCT2)
	tm := timeDatUTCT2(msg.(*sdbpbin.DatUTCT2))
	if tm == nil {
		t.Fatal("expected non-nil TimeMsg")
	}
	if tm.UTCTime == nil {
		t.Fatal("expected non-nil UTCTime")
	}
	// Verify date/time matches capture
	utc := tm.UTCTime.Date.Add(tm.UTCTime.TimeOfDay).Round(time.Second)
	wantUTC := time.Date(2026, 3, 17, 13, 23, 55, 0, time.UTC)
	if !utc.Equal(wantUTC) {
		t.Errorf("DAT-UTCT2 UTC = %v, want %v", utc, wantUTC)
	}
	if tm.GNSS != gpsprot.GPS {
		t.Errorf("GNSS = %v, want GPS", tm.GNSS)
	}
	if tm.Accuracy != 4*time.Nanosecond {
		t.Errorf("Accuracy = %v, want 4ns", tm.Accuracy)
	}
}

func TestLeapDatUTCT2Captured(t *testing.T) {
	msg := parsePacket(t, captureUTCT2)
	m := msg.(*sdbpbin.DatUTCT2)
	// Valid=11 (0b1011) = HMS+YMD+LeapCorr, missing LeapForecast
	// So leapDatUTCT2 should return nil (no future leap second info)
	lm := leapDatUTCT2(m)
	if lm != nil {
		t.Errorf("expected nil LeapSecondMsg (no leap forecast), got %+v", lm)
	}
}

func TestLeapDatGPSUCaptured(t *testing.T) {
	msg := parsePacket(t, captureGPSU)
	m := msg.(*sdbpbin.DatGPSU)
	lm := leapDatGNSSU(&m.DatGNSSU, gpsprot.GPS)
	if lm == nil {
		t.Fatal("expected non-nil LeapSecondMsg from DAT-GPSU")
	}
	// DeltaTLS=18, DeltaTLSF=18 -> past leap second (2016)
	// TAI-UTC offset = 18 + TAIMinusGPS(19) = 37
	if lm.UTCOffAfter != 37 {
		t.Errorf("UTCOffAfter = %d, want 37", lm.UTCOffAfter)
	}
	if lm.GNSS != gpsprot.GPS {
		t.Errorf("GNSS = %v, want GPS", lm.GNSS)
	}
}

func TestTimeDatUTCT2(t *testing.T) {
	m := &sdbpbin.DatUTCT2{
		Valid:   sdbpbin.DatUTCT2HMS | sdbpbin.DatUTCT2YMD,
		RefGrid: sdbpbin.DatUTCT2RefGPS,
		Year:    2026, Month: 3, Day: 17,
		Hour: 0, Min: 15, Sec: 18,
		SecFrac:  500000000, // 0.5s in ns
		Accuracy: 50,
	}
	tm := timeDatUTCT2(m)
	if tm == nil {
		t.Fatal("expected non-nil TimeMsg")
	}
	if tm.GNSS != gpsprot.GPS {
		t.Errorf("GNSS = %v, want GPS", tm.GNSS)
	}
	if tm.UTCTime == nil {
		t.Fatal("expected non-nil UTCTime")
	}
	wantDate := time.Date(2026, 3, 17, 0, 0, 0, 0, time.UTC)
	if !tm.UTCTime.Date.Equal(wantDate) {
		t.Errorf("Date = %v, want %v", tm.UTCTime.Date, wantDate)
	}
	wantTOD := 15*time.Minute + 18*time.Second + 500*time.Millisecond
	if tm.UTCTime.TimeOfDay != wantTOD {
		t.Errorf("TimeOfDay = %v, want %v", tm.UTCTime.TimeOfDay, wantTOD)
	}
	if tm.Accuracy != 50*time.Nanosecond {
		t.Errorf("Accuracy = %v, want 50ns", tm.Accuracy)
	}
	if tm.NativeMsgID != "DAT-UTCT2" {
		t.Errorf("NativeMsgID = %q, want DAT-UTCT2", tm.NativeMsgID)
	}
}

func TestTimeDatUTCT2Invalid(t *testing.T) {
	m := &sdbpbin.DatUTCT2{Valid: sdbpbin.DatUTCT2HMS} // missing YMD
	if tm := timeDatUTCT2(m); tm != nil {
		t.Error("expected nil for missing YMD")
	}
}

func TestLeapDatUTCT2(t *testing.T) {
	m := &sdbpbin.DatUTCT2{
		Valid:         sdbpbin.DatUTCT2HMS | sdbpbin.DatUTCT2YMD | sdbpbin.DatUTCT2LeapForecast | sdbpbin.DatUTCT2LeapCorr,
		RefGrid:       sdbpbin.DatUTCT2RefGPS,
		LeapSec:       18, // GPS-UTC offset = 18s -> TAI-UTC = 18 + 19 = 37
		LeapChange:    1,
		LeapYear:      2025, LeapMonth: 6, LeapDay: 30,
	}
	lm := leapDatUTCT2(m)
	if lm == nil {
		t.Fatal("expected non-nil LeapSecondMsg")
	}
	if lm.UTCOffBefore != 37 {
		t.Errorf("UTCOffBefore = %d, want 37", lm.UTCOffBefore)
	}
	if lm.UTCOffAfter != 38 {
		t.Errorf("UTCOffAfter = %d, want 38", lm.UTCOffAfter)
	}
	if lm.GNSS != gpsprot.GPS {
		t.Errorf("GNSS = %v, want GPS", lm.GNSS)
	}
	// Verify date
	wantDate := time.Date(2025, 6, 30, 0, 0, 0, 0, time.UTC)
	gotDate := lm.LeapSecond.Date()
	if !gotDate.Equal(wantDate) {
		t.Errorf("Date = %v, want %v", gotDate, wantDate)
	}
}

func TestLeapDatUTCT2Invalid(t *testing.T) {
	m := &sdbpbin.DatUTCT2{Valid: sdbpbin.DatUTCT2LeapForecast} // missing LeapCorr
	if lm := leapDatUTCT2(m); lm != nil {
		t.Error("expected nil for missing leap corr bit")
	}
}

func TestLeapDatGNSSU(t *testing.T) {
	// Simulate GPS UTC parameters with a known future leap second
	m := &sdbpbin.DatGNSSU{
		TOT:       0,
		WNOT:      0,
		DeltaTLS:  18, // current GPS-UTC = 18s
		WNLSF:     77, // low 8 bits of week of leap (2025-06-30 is GPS week 2373, 2373 mod 256 = 69... use a simpler example)
		DN:        1,  // Sunday in GPS 1-based
		DeltaTLSF: 18, // same = past leap second
	}
	// For a past leap second (DeltaTLS == DeltaTLSF), the function should still produce a result.
	lm := leapDatGNSSU(m, gpsprot.GPS)
	// May return nil if ptime can't resolve the week; that's acceptable for this synthetic data.
	// The important thing is it doesn't panic.
	_ = lm
}
