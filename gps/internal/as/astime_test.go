package as

import (
	"encoding/hex"
	"reflect"
	"testing"
	"time"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/asbin"
	"github.com/jclark/satpulse/gps/lib/opt"
	"github.com/jclark/satpulse/gps/ptime"
)

func TestTimeNavTimeTAU1201Rollover(t *testing.T) {
	// Captured from a Star River SR1723TAU1201 board running firmware 3.M6A.a3f23db.
	// Both periodic and polled NAV-TIME report week 388 instead of 2436.
	tests := []struct {
		name     string
		packet   string
		utc      ptime.UTCTime
		accuracy time.Duration
	}{
		{
			name:     "periodic NavSys 24",
			packet:   "f1d90105100018075fec68ba3e1c840112000d000000a0b7",
			utc:      ptime.UTC(2026, 9, 18, 11, 37, 35, 0),
			accuracy: 13 * time.Nanosecond,
		},
		{
			name:     "polled GPS",
			packet:   "f1d90105100000072ffd90fc8f1f840112001b00000035a7",
			utc:      ptime.UTC(2026, 9, 19, 3, 5, 12, 0),
			accuracy: 27 * time.Nanosecond,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			packet, err := hex.DecodeString(tc.packet)
			if err != nil {
				t.Fatal(err)
			}
			msg, err := asbin.ParseMsg(string(packet))
			if err != nil {
				t.Fatal(err)
			}
			m, ok := msg.(*asbin.NavTime)
			if !ok {
				t.Fatalf("parsed %T, want *asbin.NavTime", msg)
			}
			if m.Week != 388 {
				t.Fatalf("packet week = %d, want 388", m.Week)
			}
			want := gpsprot.TimeMsg{
				NativeMsgID: "NAV-TIME",
				TAITime:     ptime.LeapSecond2016().UTCtoTime(tc.utc),
				GNSS:        gpsprot.GPS,
				UTCOffset:   37,
				Accuracy:    tc.accuracy,
			}
			if got := timeNavTime(m); !reflect.DeepEqual(*got, want) {
				t.Errorf("timeNavTime() = %+v, want %+v", *got, want)
			}
		})
	}
}

// TestTimeNavTimePackets checks the time of week rounding (see navTowGrid)
// on packets copied from gps/testdata/packets/allystar.
func TestTimeNavTimePackets(t *testing.T) {
	tests := []struct {
		name   string
		packet string
		expect gpsprot.TimeMsg
	}{
		{
			name:   "TAU1201 NAV-TIME refTow ending in 999", // coldstart.jsonl
			packet: "f1d9010510001807d38097501a027a0912000b0000002bc7",
			expect: gpsprot.TimeMsg{
				NativeMsgID: "NAV-TIME",
				TAITime:     ptime.GPS(2426, 35279000*time.Millisecond),
				GNSS:        gpsprot.GPS,
				UTCOffset:   37,
				Accuracy:    11 * time.Nanosecond,
			},
		},
		{
			name:   "TAU1201 NAV-TIMEUTC iTow ending in 999", // coldstart.jsonl
			packet: "f1d90121140097501a0200000000d3800e00ea070705092f29271f57",
			expect: gpsprot.TimeMsg{
				NativeMsgID: "NAV-TIMEUTC",
				UTCTime:     utcTimePtr(ptime.UTC(2026, 7, 5, 9, 47, 41, 0)),
				GNSS:        gpsprot.GPS,
			},
		},
		{
			name:   "TAU951M-P200 NAV-TIME Fractow in 100ns", // daemon.jsonl
			packet: "f1d9010510001a07f611f086b3077b091200090000000dd3",
			expect: gpsprot.TimeMsg{
				NativeMsgID: "NAV-TIME",
				TAITime:     ptime.GPS(2427, 129206000*time.Millisecond),
				GNSS:        gpsprot.GPS,
				UTCOffset:   37,
				Accuracy:    9 * time.Nanosecond,
			},
		},
		{
			name:   "TAU951M-P200 NAV-TIMEUTC at 5Hz", // daemon-5hz.jsonl
			packet: "f1d90121140028dc1c020b0000007bae0200ea07070509321b270832",
			expect: gpsprot.TimeMsg{
				NativeMsgID: "NAV-TIMEUTC",
				UTCTime:     utcTimePtr(ptime.UTC(2026, 7, 5, 9, 50, 27, 800000000)),
				Accuracy:    11 * time.Nanosecond,
				GNSS:        gpsprot.GPS,
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			packet, err := hex.DecodeString(tc.packet)
			if err != nil {
				t.Fatal(err)
			}
			msg, err := asbin.ParseMsg(string(packet))
			if err != nil {
				t.Fatal(err)
			}
			var got *gpsprot.TimeMsg
			switch m := msg.(type) {
			case *asbin.NavTime:
				got = timeNavTime(m)
			case *asbin.NavTimeUTC:
				got = timeNavTimeUTC(m)
			default:
				t.Fatalf("parsed %T, want *asbin.NavTime or *asbin.NavTimeUTC", msg)
			}
			if !reflect.DeepEqual(*got, tc.expect) {
				t.Errorf("got  %+v\nwant %+v", *got, tc.expect)
			}
		})
	}
}

func TestTimeNavTime(t *testing.T) {
	tests := []struct {
		name   string
		input  asbin.NavTime
		expect gpsprot.TimeMsg
	}{
		{
			name: "invalid week",
			input: asbin.NavTime{
				NavSys:  asbin.NavTimeSysGPS,
				Flags:   asbin.NavTimeFlagSecondValid, // week not valid
				RefTow:  100000,
				Week:    2345,
				LeapSec: 18,
			},
			expect: gpsprot.TimeMsg{NativeMsgID: "NAV-TIME"},
		},
		{
			name: "invalid second",
			input: asbin.NavTime{
				NavSys:  asbin.NavTimeSysGPS,
				Flags:   asbin.NavTimeFlagWeekValid, // second not valid
				RefTow:  100000,
				Week:    2345,
				LeapSec: 18,
			},
			expect: gpsprot.TimeMsg{NativeMsgID: "NAV-TIME"},
		},
		{
			name: "GPS time valid",
			input: asbin.NavTime{
				NavSys:  asbin.NavTimeSysGPS,
				Flags:   asbin.NavTimeFlagWeekValid | asbin.NavTimeFlagSecondValid | asbin.NavTimeFlagLeapSecValid,
				RefTow:  0,
				Fractow: 0,
				Week:    2345,
				LeapSec: 18,
				TimeErr: 50,
			},
			expect: gpsprot.TimeMsg{
				NativeMsgID: "NAV-TIME",
				TAITime:     ptime.GPS(2345, 0),
				GNSS:        gpsprot.GPS,
				UTCOffset:   18 + ptime.TAIMinusGPS,
				Accuracy:    50 * time.Nanosecond,
			},
		},
		{
			name: "GPS time without leap second",
			input: asbin.NavTime{
				NavSys:  asbin.NavTimeSysGPS,
				Flags:   asbin.NavTimeFlagWeekValid | asbin.NavTimeFlagSecondValid,
				RefTow:  100000,
				Week:    2345,
				LeapSec: 18,
			},
			expect: gpsprot.TimeMsg{
				NativeMsgID: "NAV-TIME",
				TAITime:     ptime.GPS(2345, 100000*time.Millisecond),
				GNSS:        gpsprot.GPS,
				UTCOffset:   0, // leap second not valid
			},
		},
		{
			name: "Galileo time",
			input: asbin.NavTime{
				NavSys:  asbin.NavTimeSysGalileo,
				Flags:   asbin.NavTimeFlagWeekValid | asbin.NavTimeFlagSecondValid | asbin.NavTimeFlagLeapSecValid,
				RefTow:  0,
				Week:    1412,
				LeapSec: 18,
			},
			expect: gpsprot.TimeMsg{
				NativeMsgID: "NAV-TIME",
				TAITime:     ptime.Galileo(1412, 0),
				GNSS:        gpsprot.GAL,
				UTCOffset:   18 + ptime.TAIMinusGalileo,
			},
		},
		{
			name: "BeiDou time",
			input: asbin.NavTime{
				NavSys:  asbin.NavTimeSysBeiDou,
				Flags:   asbin.NavTimeFlagWeekValid | asbin.NavTimeFlagSecondValid | asbin.NavTimeFlagLeapSecValid,
				RefTow:  0,
				Week:    900,
				LeapSec: 4,
			},
			expect: gpsprot.TimeMsg{
				NativeMsgID: "NAV-TIME",
				TAITime:     ptime.BeiDou(900, 0),
				GNSS:        gpsprot.BDS,
				UTCOffset:   4 + ptime.TAIMinusBeiDou,
			},
		},
		{
			name: "GLONASS treated as GPS",
			input: asbin.NavTime{
				NavSys:  asbin.NavTimeSysGLONASS,
				Flags:   asbin.NavTimeFlagWeekValid | asbin.NavTimeFlagSecondValid | asbin.NavTimeFlagLeapSecValid,
				RefTow:  0,
				Week:    2345,
				LeapSec: 18,
			},
			expect: gpsprot.TimeMsg{
				NativeMsgID: "NAV-TIME",
				TAITime:     ptime.GPS(2345, 0),
				GNSS:        gpsprot.GPS,
				UTCOffset:   18 + ptime.TAIMinusGPS,
			},
		},
		{
			name: "undocumented NavSys 24 treated as GPS",
			input: asbin.NavTime{
				NavSys:  24, // TAU1201 firmware 3.018 sends 24 (0x18) for periodic messages
				Flags:   asbin.NavTimeFlagWeekValid | asbin.NavTimeFlagSecondValid | asbin.NavTimeFlagLeapSecValid,
				RefTow:  0,
				Week:    2345,
				LeapSec: 18,
			},
			expect: gpsprot.TimeMsg{
				NativeMsgID: "NAV-TIME",
				TAITime:     ptime.GPS(2345, 0),
				GNSS:        gpsprot.GPS,
				UTCOffset:   18 + ptime.TAIMinusGPS,
			},
		},
		{
			name: "week overflow",
			input: asbin.NavTime{
				NavSys: asbin.NavTimeSysGPS,
				Flags:  asbin.NavTimeFlagWeekValid | asbin.NavTimeFlagSecondValid,
				RefTow: 0,
				Week:   0x8000, // > math.MaxInt16
			},
			expect: gpsprot.TimeMsg{NativeMsgID: "NAV-TIME"},
		},
		{
			name: "accuracy in nanoseconds",
			input: asbin.NavTime{
				NavSys:  asbin.NavTimeSysGPS,
				Flags:   asbin.NavTimeFlagWeekValid | asbin.NavTimeFlagSecondValid,
				RefTow:  100000,
				Week:    2345,
				TimeErr: 12345,
			},
			expect: gpsprot.TimeMsg{
				NativeMsgID: "NAV-TIME",
				TAITime:     ptime.GPS(2345, 100000*time.Millisecond),
				GNSS:        gpsprot.GPS,
				Accuracy:    12345 * time.Nanosecond,
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := timeNavTime(&tc.input)
			if !reflect.DeepEqual(*got, tc.expect) {
				t.Errorf("timeNavTime() = %+v, want %+v", *got, tc.expect)
			}
		})
	}
}

func TestTimeNavTimeUTC(t *testing.T) {
	allValid := asbin.NavTimeUTCFlagTowValid | asbin.NavTimeUTCFlagWknValid | asbin.NavTimeUTCFlagUtcValid
	tests := []struct {
		name   string
		input  asbin.NavTimeUTC
		expect gpsprot.TimeMsg
	}{
		{
			name: "invalid missing UTC flag",
			input: asbin.NavTimeUTC{
				ValidFlag: asbin.NavTimeUTCFlagTowValid | asbin.NavTimeUTCFlagWknValid,
			},
			expect: gpsprot.TimeMsg{NativeMsgID: "NAV-TIMEUTC"},
		},
		{
			name: "invalid missing TOW flag",
			input: asbin.NavTimeUTC{
				ValidFlag: asbin.NavTimeUTCFlagWknValid | asbin.NavTimeUTCFlagUtcValid,
			},
			expect: gpsprot.TimeMsg{NativeMsgID: "NAV-TIMEUTC"},
		},
		{
			name: "invalid missing WKN flag",
			input: asbin.NavTimeUTC{
				ValidFlag: asbin.NavTimeUTCFlagTowValid | asbin.NavTimeUTCFlagUtcValid,
			},
			expect: gpsprot.TimeMsg{NativeMsgID: "NAV-TIMEUTC"},
		},
		{
			name: "valid with USNO",
			input: asbin.NavTimeUTC{
				Year:      2024,
				Month:     3,
				Day:       15,
				Hour:      12,
				Min:       30,
				Sec:       45,
				TAcc:      50,
				ValidFlag: allValid | asbin.NavTimeUTCFlags(asbin.UTCStandardUSNO<<4),
			},
			expect: gpsprot.TimeMsg{
				NativeMsgID: "NAV-TIMEUTC",
				UTCTime:     utcTimePtr(ptime.UTC(2024, 3, 15, 12, 30, 45, 0)),
				Accuracy:    50 * time.Nanosecond,
				GNSS:        gpsprot.GPS,
			},
		},
		{
			name: "valid with NTSC",
			input: asbin.NavTimeUTC{
				Year:      2024,
				Month:     1,
				Day:       1,
				Hour:      0,
				Min:       0,
				Sec:       0,
				Nano:      0,
				TAcc:      100,
				ValidFlag: allValid | asbin.NavTimeUTCFlags(asbin.UTCStandardNTSC<<4),
			},
			expect: gpsprot.TimeMsg{
				NativeMsgID: "NAV-TIMEUTC",
				UTCTime:     utcTimePtr(ptime.UTC(2024, 1, 1, 0, 0, 0, 0)),
				Accuracy:    100 * time.Nanosecond,
				GNSS:        gpsprot.BDS,
			},
		},
		{
			name: "valid with EUL",
			input: asbin.NavTimeUTC{
				Year:      2024,
				Month:     6,
				Day:       15,
				Hour:      18,
				Min:       45,
				Sec:       30,
				TAcc:      200,
				ValidFlag: allValid | asbin.NavTimeUTCFlags(asbin.UTCStandardEU<<4),
			},
			expect: gpsprot.TimeMsg{
				NativeMsgID: "NAV-TIMEUTC",
				UTCTime:     utcTimePtr(ptime.UTC(2024, 6, 15, 18, 45, 30, 0)),
				Accuracy:    200 * time.Nanosecond,
				GNSS:        gpsprot.GAL,
			},
		},
		{
			name: "valid with SU",
			input: asbin.NavTimeUTC{
				Year:      2024,
				Month:     12,
				Day:       31,
				Hour:      23,
				Min:       59,
				Sec:       59,
				TAcc:      300,
				ValidFlag: allValid | asbin.NavTimeUTCFlags(asbin.UTCStandardSU<<4),
			},
			expect: gpsprot.TimeMsg{
				NativeMsgID: "NAV-TIMEUTC",
				UTCTime:     utcTimePtr(ptime.UTC(2024, 12, 31, 23, 59, 59, 0)),
				Accuracy:    300 * time.Nanosecond,
				GNSS:        gpsprot.GLO,
			},
		},
		{
			name: "valid with unknown standard",
			input: asbin.NavTimeUTC{
				Year:      2024,
				Month:     1,
				Day:       1,
				Hour:      0,
				Min:       0,
				Sec:       0,
				Nano:      0,
				TAcc:      50,
				ValidFlag: allValid, // 0 = not available
			},
			expect: gpsprot.TimeMsg{
				NativeMsgID: "NAV-TIMEUTC",
				UTCTime:     utcTimePtr(ptime.UTC(2024, 1, 1, 0, 0, 0, 0)),
				Accuracy:    50 * time.Nanosecond,
				GNSS:        0,
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := timeNavTimeUTC(&tc.input)
			if !reflect.DeepEqual(*got, tc.expect) {
				t.Errorf("timeNavTimeUTC() = %+v, want %+v", *got, tc.expect)
			}
		})
	}
}

func utcTimePtr(u ptime.UTCTime) opt.Val[ptime.UTCTime] {
	return opt.Make(u)
}
