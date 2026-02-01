package as

import (
	"reflect"
	"testing"
	"time"

	"github.com/jclark/satpulse/internal/asbin"
	"github.com/jclark/satpulse/internal/gpsprot"
	"github.com/jclark/satpulse/internal/ptime"
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
