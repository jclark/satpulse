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
		LeapSec: 18,
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
	// LeapSec 18 (GPS-UTC) + TAIMinusGPS (19) = 37
	if tm.UTCOffset != 37 {
		t.Errorf("UTCOffset = %v, want 37", tm.UTCOffset)
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

// timeMsgHandler captures TimeMsg for test verification.
type timeMsgHandler struct {
	gpsprot.DefaultHandler
	times []*gpsprot.TimeMsg
}

func (h *timeMsgHandler) Time(msg *gpsprot.TimeMsg, _ time.Time) {
	h.times = append(h.times, msg)
}

// TestNav2SolGNSSFromNav2TimeUTC verifies that the processor propagates
// the GNSS source from Nav2TimeUTC to Nav2Sol's TimeMsg.
func TestNav2SolGNSSFromNav2TimeUTC(t *testing.T) {
	mgr := gpsprot.NewNavEpochManager()
	pp := NewPacketProcessor(mgr)
	h := &timeMsgHandler{}
	pp.SetMsgHandler(h)
	tRead := time.Unix(1, 0)
	serialize := func(m casbin.Msg) string {
		pkt, err := casbin.Serialize(m)
		if err != nil {
			t.Fatalf("serialize %T: %v", m, err)
		}
		return string(pkt)
	}
	tests := []struct {
		name    string
		timeSrc casbin.Nav2TimeSrc
		want    gpsprot.GNSS
	}{
		{"GPS", casbin.Nav2TimeSrcGPS, gpsprot.GPS},
		{"BDS", casbin.Nav2TimeSrcBDS, gpsprot.BDS},
		{"GAL", casbin.Nav2TimeSrcGAL, gpsprot.GAL},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h.times = nil
			// Send Nav2TimeUTC to set the time source
			pp.ProcessPacket(serialize(&casbin.Nav2TimeUTC{
				TFlags:  casbin.Nav2TimeTOWValid | casbin.Nav2TimeReliable,
				TimeSrc: tt.timeSrc,
				Year:    2026, Month: 3, Day: 5,
			}), tRead)
			// Send Nav2Sol with a valid fix in the same epoch
			pp.ProcessPacket(serialize(&casbin.Nav2Sol{
				Nav2TOW:  casbin.Nav2TOW{TOW: 259200000},
				Wn:       2356,
				FixFlags: casbin.Nav2Fix3D,
			}), tRead)
			// Find the NAV2-SOL TimeMsg
			var solMsg *gpsprot.TimeMsg
			for _, tm := range h.times {
				if tm.NativeMsgID == "NAV2-SOL" {
					solMsg = tm
				}
			}
			if solMsg == nil {
				t.Fatal("no NAV2-SOL TimeMsg emitted")
			}
			if solMsg.GNSS != tt.want {
				t.Errorf("GNSS = %v, want %v", solMsg.GNSS, tt.want)
			}
			if solMsg.TAITime == 0 {
				t.Error("TAITime is zero")
			}
		})
	}
}

// TestNav2SolGNSSWithoutNav2TimeUTC verifies that when no Nav2TimeUTC
// has been received, Nav2Sol's TimeMsg has zero (unknown) GNSS.
func TestNav2SolGNSSWithoutNav2TimeUTC(t *testing.T) {
	mgr := gpsprot.NewNavEpochManager()
	pp := NewPacketProcessor(mgr)
	h := &timeMsgHandler{}
	pp.SetMsgHandler(h)
	pkt, err := casbin.Serialize(&casbin.Nav2Sol{
		Nav2TOW:  casbin.Nav2TOW{TOW: 259200000},
		Wn:       2356,
		FixFlags: casbin.Nav2Fix3D,
	})
	if err != nil {
		t.Fatal(err)
	}
	pp.ProcessPacket(string(pkt), time.Unix(1, 0))
	if len(h.times) != 1 {
		t.Fatalf("got %d TimeMsgs, want 1", len(h.times))
	}
	if h.times[0].GNSS != 0 {
		t.Errorf("GNSS = %v, want 0 (unknown)", h.times[0].GNSS)
	}
}

