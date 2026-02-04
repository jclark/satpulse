package as

import (
	"reflect"
	"testing"
	"time"

	"github.com/jclark/satpulse/gps/lib/asbin"
	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/ptime"
)

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
				Fractow: 500,
				Week:    2345,
				LeapSec: 18,
			},
			expect: gpsprot.TimeMsg{
				NativeMsgID: "NAV-TIME",
				TAITime:     ptime.GPS(2345, 100000*time.Millisecond+500*time.Nanosecond),
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
				Week:    2345,
				LeapSec: 18,
			},
			expect: gpsprot.TimeMsg{
				NativeMsgID: "NAV-TIME",
				TAITime:     ptime.Galileo(2345, 0),
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
				NavSys:  asbin.NavTimeSysGPS,
				Flags:   asbin.NavTimeFlagWeekValid | asbin.NavTimeFlagSecondValid,
				RefTow:  0,
				Week:    0x8000, // > math.MaxInt16
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
		{
			name: "fractional tow in nanoseconds",
			input: asbin.NavTime{
				NavSys:  asbin.NavTimeSysGPS,
				Flags:   asbin.NavTimeFlagWeekValid | asbin.NavTimeFlagSecondValid,
				RefTow:  1000,  // 1000 ms = 1 second
				Fractow: 12345, // 12345 ns
				Week:    2345,
			},
			expect: gpsprot.TimeMsg{
				NativeMsgID: "NAV-TIME",
				TAITime:     ptime.GPS(2345, 1000*time.Millisecond+12345*time.Nanosecond),
				GNSS:        gpsprot.GPS,
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
				Nano:      123456789,
				TAcc:      50,
				ValidFlag: allValid | asbin.NavTimeUTCFlags(asbin.UTCStandardUSNO<<4),
			},
			expect: gpsprot.TimeMsg{
				NativeMsgID: "NAV-TIMEUTC",
				UTCTime:     utcTimePtr(ptime.UTC(2024, 3, 15, 12, 30, 45, 123456789)),
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
				Nano:      500000000,
				TAcc:      200,
				ValidFlag: allValid | asbin.NavTimeUTCFlags(asbin.UTCStandardEU<<4),
			},
			expect: gpsprot.TimeMsg{
				NativeMsgID: "NAV-TIMEUTC",
				UTCTime:     utcTimePtr(ptime.UTC(2024, 6, 15, 18, 45, 30, 500000000)),
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
				Nano:      999999999,
				TAcc:      300,
				ValidFlag: allValid | asbin.NavTimeUTCFlags(asbin.UTCStandardSU<<4),
			},
			expect: gpsprot.TimeMsg{
				NativeMsgID: "NAV-TIMEUTC",
				UTCTime:     utcTimePtr(ptime.UTC(2024, 12, 31, 23, 59, 59, 999999999)),
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

func utcTimePtr(u ptime.UTCTime) *ptime.UTCTime {
	return &u
}
