package sdbp

import (
	"encoding/hex"
	"reflect"
	"testing"
	"time"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/opt"
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

func optVal[T any](v T) opt.Val[T] { return opt.Make(v) }

// Adjacent pair from taidou-capture2.jsonl:
//
//	{"t":"2026-03-17T00:15:18.054733Z","tag":"SDBP","msg":"DAT-TPPS","bin":"233e064111000008000000b83405416a094d0100000001540f"}
//	{"t":"2026-03-17T00:15:18.056741Z","tag":"SDBP","msg":"DAT-GPST","bin":"233e0617130040005b0a036a09ad8d24ff3f350541030000006550"}
//
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
				UTCTime:     optVal(ptime.GPSUTC(2410, ptime.Seconds(134202.0))),
				PulseOffset: optVal(597e-3),
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
				UTCTime:     optVal(ptime.GPSUTC(2410, ptime.Seconds(173719.0))),
				PulseOffset: optVal(333e-3),
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
	utc := tm.UTCTime.Get().SysTime().Round(time.Second)
	wantUTC := captureTime.Add(time.Second)
	if !utc.Equal(wantUTC) {
		t.Errorf("DAT-TPPS UTC = %v, want %v (capture time + 1s)", utc, wantUTC)
	}
}

func TestTimeDatUTCT2(t *testing.T) {
	utcTime := func(y int, mo time.Month, d int, tod time.Duration) opt.Val[ptime.UTCTime] {
		return opt.Make(ptime.UTCTime{Date: time.Date(y, mo, d, 0, 0, 0, 0, time.UTC), TimeOfDay: tod})
	}
	tests := []struct {
		name  string
		input *sdbpbin.DatUTCT2
		want  *gpsprot.TimeMsg
	}{
		{
			name:  "captured",
			input: parsePacket(t, "233e061f1f0048002d0d0b02ea0703110d173741a002000400000012ffffffff000000000028d8").(*sdbpbin.DatUTCT2),
			want: &gpsprot.TimeMsg{
				Ref:         gpsprot.NavSolution,
				NativeMsgID: "DAT-UTCT2",
				GNSS:        gpsprot.GPS,
				UTCTime:     utcTime(2026, 3, 17, 13*time.Hour+23*time.Minute+55*time.Second+172097*time.Nanosecond),
				Accuracy:    4 * time.Nanosecond,
			},
		},
		{
			name: "synthetic",
			input: &sdbpbin.DatUTCT2{
				Valid: sdbpbin.DatUTCT2HMS | sdbpbin.DatUTCT2YMD, RefGrid: sdbpbin.DatUTCT2RefGPS,
				Year: 2026, Month: 3, Day: 17, Hour: 0, Min: 15, Sec: 18,
				SecFrac: 500000000, Accuracy: 50,
			},
			want: &gpsprot.TimeMsg{
				Ref:         gpsprot.NavSolution,
				NativeMsgID: "DAT-UTCT2",
				GNSS:        gpsprot.GPS,
				UTCTime:     utcTime(2026, 3, 17, 15*time.Minute+18*time.Second+500*time.Millisecond),
				Accuracy:    50 * time.Nanosecond,
			},
		},
		{
			name:  "missing-YMD",
			input: &sdbpbin.DatUTCT2{Valid: sdbpbin.DatUTCT2HMS},
			want:  nil,
		},
		{
			name:  "missing-HMS",
			input: &sdbpbin.DatUTCT2{Valid: sdbpbin.DatUTCT2YMD},
			want:  nil,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := timeDatUTCT2(tc.input)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("timeDatUTCT2:\n  got  %+v\n  want %+v", got, tc.want)
			}
		})
	}
}

func TestLeapSecondDatUTCT2(t *testing.T) {
	tests := []struct {
		name  string
		input *sdbpbin.DatUTCT2
		want  *gpsprot.LeapSecondMsg
	}{
		{
			name: "future-leap",
			input: &sdbpbin.DatUTCT2{
				Valid:   sdbpbin.DatUTCT2HMS | sdbpbin.DatUTCT2YMD | sdbpbin.DatUTCT2LeapForecast | sdbpbin.DatUTCT2LeapCorr,
				RefGrid: sdbpbin.DatUTCT2RefGPS,
				LeapSec: 18, LeapChange: 1,
				LeapYear: 2025, LeapMonth: 6, LeapDay: 30,
			},
			want: &gpsprot.LeapSecondMsg{
				LeapSecond: ptime.LeapSecondOnDate(time.Date(2025, 6, 30, 0, 0, 0, 0, time.UTC), 37, 38),
				GNSS:       gpsprot.GPS,
			},
		},
		{
			name: "captured-no-forecast",
			// Valid=11 (0b1011) = HMS+YMD+LeapCorr, missing LeapForecast
			input: parsePacket(t, "233e061f1f0048002d0d0b02ea0703110d173741a002000400000012ffffffff000000000028d8").(*sdbpbin.DatUTCT2),
			want:  nil,
		},
		{
			name:  "missing-corr",
			input: &sdbpbin.DatUTCT2{Valid: sdbpbin.DatUTCT2LeapForecast},
			want:  nil,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := leapSecondDatUTCT2(tc.input)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("leapSecondDatUTCT2:\n  got  %+v\n  want %+v", got, tc.want)
			}
		})
	}
}

func TestLeapSecondDatGPSUCaptured(t *testing.T) {
	m := parsePacket(t, "233e062d230000000000000030be000000000000e0bc0000000000000000003006006a09128909071246da").(*sdbpbin.DatGPSU)
	got := leapSecondDatGNSSU(&m.DatGNSSU, gpsprot.GPS)
	want := &gpsprot.LeapSecondMsg{
		LeapSecond: ptime.LeapSecond2016(),
		GNSS:       gpsprot.GPS,
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("leapSecondDatGNSSU:\n  got  %+v\n  want %+v", got, want)
	}
}
