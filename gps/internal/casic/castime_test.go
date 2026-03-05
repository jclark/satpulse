package casic

import (
	"testing"
	"time"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/casbin"
)

func TestTimeNav2TimeUTC(t *testing.T) {
	m := &casbin.Nav2TimeUTC{
		TAcc:    50.0, // 50 ns
		Subms:   0,
		Subcs:   0,
		Cs:      50, // 500 ms
		Year:    2026,
		Month:   3,
		Day:     4,
		Hour:    12,
		Min:     30,
		Sec:     45,
		TFlags:  casbin.Nav2TimeTOWValid | casbin.Nav2TimeReliable,
		TimeSrc: casbin.Nav2TimeSrcGPS,
	}
	tm := timeNav2TimeUTC(m)
	if tm == nil {
		t.Fatal("timeNav2TimeUTC() returned nil")
	}
	if tm.NativeMsgID != "NAV2-TIMEUTC" {
		t.Errorf("NativeMsgID = %v, want NAV2-TIMEUTC", tm.NativeMsgID)
	}
	if tm.UTCTime == nil {
		t.Fatal("UTCTime is nil")
	}
	if tm.GNSS != gpsprot.GPS {
		t.Errorf("GNSS = %v, want GPS", tm.GNSS)
	}
	// TAcc is 50 ns
	if tm.Accuracy != 50*time.Nanosecond {
		t.Errorf("Accuracy = %v, want 50ns", tm.Accuracy)
	}
}

func TestTimeNav2TimeUTCInvalid(t *testing.T) {
	// Missing Nav2TimeReliable flag
	m := &casbin.Nav2TimeUTC{
		TFlags:  casbin.Nav2TimeTOWValid,
		TimeSrc: casbin.Nav2TimeSrcGPS,
	}
	tm := timeNav2TimeUTC(m)
	if tm == nil {
		t.Fatal("timeNav2TimeUTC() returned nil")
	}
	if tm.UTCTime != nil {
		t.Error("UTCTime should be nil when TFlags missing Reliable")
	}
}

func TestTimeNav2TimeUTCGalileoSrc(t *testing.T) {
	m := &casbin.Nav2TimeUTC{
		TFlags:  casbin.Nav2TimeTOWValid | casbin.Nav2TimeReliable,
		Cs:      0,
		Year:    2026,
		Month:   1,
		Day:     1,
		Hour:    0,
		Min:     0,
		Sec:     0,
		TimeSrc: casbin.Nav2TimeSrcGAL,
	}
	tm := timeNav2TimeUTC(m)
	if tm.GNSS != gpsprot.GAL {
		t.Errorf("GNSS = %v, want GAL", tm.GNSS)
	}
}
