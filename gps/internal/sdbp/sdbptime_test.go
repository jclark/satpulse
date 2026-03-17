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
		valid sdbpbin.DatGPSTValid
	}{
		{"no flags", 0},
		{"tow only", sdbpbin.DatGPSTTowValid},
		{"week only", sdbpbin.DatGPSTWeekValid},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gpst := &sdbpbin.DatGPST{Valid: tc.valid}
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
