package ubx

import (
	"testing"
	"time"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/ubxbin"
	"github.com/jclark/satpulse/gps/ptime"
)

func TestMSScaledTOW(t *testing.T) {
	ms := uint32(0x80000000)
	expected := time.Millisecond / 2
	result := msScaledTOW(ms)
	if result != expected {
		t.Errorf("scaledMSToNS(0x%X) = %d; expected %d", ms, result, expected)
	}
}

func TestTimeTimTPGLONASS(t *testing.T) {
	// GLONASS time = UTC + 3h. When GNSS ref is GLONASS, the week/TOW
	// are in GLONASS time. We should get a UTCTime with 3h subtracted,
	// not a TAITime.
	var week uint16 = 2410
	var towMS uint32 = 10_800_000 // 3h in ms
	m := &ubxbin.TimTP{
		TOWMS:   towMS,
		Week:    week,
		Flags:   ubxbin.TimTPTimeBaseGNSS,
		RefInfo: ubxbin.TimTPTimeRefGLONASS,
	}
	got := timeTimTP(m)
	if got == nil {
		t.Fatal("timeTimTP returned nil")
	}
	if got.GNSS != gpsprot.GLO {
		t.Errorf("GNSS = %v, want GLO", got.GNSS)
	}
	if !got.TAITime.IsZero() {
		t.Errorf("TAITime = %v, want zero", got.TAITime)
	}
	if got.UTCTime == nil {
		t.Fatal("UTCTime is nil, want non-nil")
	}
	// GPSUTC(2410, 3h) gives Date=<start of week Sunday>, TOD=3h.
	// After subtracting 3h for Moscow offset, TOD should be 0.
	want := ptime.GPSUTC(week, time.Duration(towMS)*time.Millisecond)
	want.TimeOfDay -= 3 * time.Hour
	if *got.UTCTime != want {
		t.Errorf("UTCTime = %+v, want %+v", *got.UTCTime, want)
	}
}

func TestTimeTimTPGPS(t *testing.T) {
	// GPS reference should produce TAITime, not UTCTime.
	m := &ubxbin.TimTP{
		TOWMS:   10_800_000,
		Week:    2410,
		Flags:   ubxbin.TimTPTimeBaseGNSS,
		RefInfo: ubxbin.TimTPTimeRefGPS,
	}
	got := timeTimTP(m)
	if got == nil {
		t.Fatal("timeTimTP returned nil")
	}
	if got.GNSS != gpsprot.GPS {
		t.Errorf("GNSS = %v, want GPS", got.GNSS)
	}
	if got.TAITime.IsZero() {
		t.Error("TAITime is zero, want non-zero")
	}
	if got.UTCTime != nil {
		t.Errorf("UTCTime = %+v, want nil", *got.UTCTime)
	}
	want := ptime.GPS(int16(m.Week), time.Duration(m.TOWMS)*time.Millisecond)
	if got.TAITime != want {
		t.Errorf("TAITime = %v, want %v", got.TAITime, want)
	}
}
