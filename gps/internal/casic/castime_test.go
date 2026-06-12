package casic

import (
	"encoding/hex"
	"math"
	"reflect"
	"testing"
	"time"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/casbin"
	"github.com/jclark/satpulse/gps/lib/opt"
	"github.com/jclark/satpulse/gps/ptime"
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
	if !tm.UTCTime.IsSet() {
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
	if tm.UTCTime.IsSet() {
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
				FixFlags: casbin.PVT3D,
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
		FixFlags: casbin.PVT3D,
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

func parseTimHex(t *testing.T, hexStr string) *casbin.Tim2TimeGNSS {
	t.Helper()
	b, err := hex.DecodeString(hexStr)
	if err != nil {
		t.Fatalf("bad hex: %v", err)
	}
	msg, err := casbin.ParseMsg(string(b))
	if err != nil {
		t.Fatalf("ParseMsg: %v", err)
	}
	switch m := msg.(type) {
	case *casbin.Tim2TimeGPS:
		return &m.Tim2TimeGNSS
	case *casbin.Tim2TimeBDS:
		return &m.Tim2TimeGNSS
	case *casbin.Tim2TimeGLN:
		return &m.Tim2TimeGNSS
	case *casbin.Tim2TimeGAL:
		return &m.Tim2TimeGNSS
	default:
		t.Fatalf("unexpected type %T", msg)
		return nil
	}
}

// Captured at 2026-03-22T03:49:56Z (UTC) = 2026-03-22T03:50:33 TAI
var tim2CapturedHex = map[string]string{
	"GPS": "bace24001201f0c8d2003e81ffff6b0907037629ea56df8810410c3f93bf9cdf1d0112120f0001010000cd37a65d",
	"BDS": "bace240012024092d200da52ffff1f040703684b0826e07b5540000000009cdf1d0104040f01010100004695756e",
	"GLN": "bace24001203a082d200da52ffff290604036494d9384e2c7a4400000000000000000000030200000000799c3e86",
	"GAL": "bace24001204f0c8d200da52ffff6b050403762900324e2c7a44000000009cdf1d011212030303010000ce698382",
}

// TestTimeTim2TimeGNSSAgree verifies that GPS, BDS, and GAL produce the same TAI time.
func TestTimeTim2TimeGNSSAgree(t *testing.T) {
	type tc struct {
		name          string
		gnss          gpsprot.GNSS
		toTAI         func(int16, time.Duration) ptime.Time
		taiMinusGNSS  int16
		msgID         string
		wantUTCOffset uint8
	}
	tests := []tc{
		{"GPS", gpsprot.GPS, ptime.GPS, ptime.TAIMinusGPS, "TIM2-TIMEGPS", 37},
		{"BDS", gpsprot.BDS, ptime.BeiDou, ptime.TAIMinusBeiDou, "TIM2-TIMEBDS", 37},
		{"GAL", gpsprot.GAL, ptime.Galileo, ptime.TAIMinusGalileo, "TIM2-TIMEGAL", 0},
	}
	var taiTimes []ptime.Time
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := parseTimHex(t, tim2CapturedHex[tt.name])
			tm := timeTim2TimeGNSS(m, tt.gnss, tt.toTAI, tt.taiMinusGNSS, tt.msgID)
			if tm.TAITime.IsZero() {
				t.Fatal("TAITime is zero")
			}
			if tm.NativeMsgID != tt.msgID {
				t.Errorf("NativeMsgID = %v, want %v", tm.NativeMsgID, tt.msgID)
			}
			if tm.GNSS != tt.gnss {
				t.Errorf("GNSS = %v, want %v", tm.GNSS, tt.gnss)
			}
			if tm.UTCOffset != tt.wantUTCOffset {
				t.Errorf("UTCOffset = %v, want %v", tm.UTCOffset, tt.wantUTCOffset)
			}
			taiTimes = append(taiTimes, tm.TAITime)
		})
	}
	// GPS, BDS, GAL TAI times should agree within 1 microsecond
	for i := 1; i < len(taiTimes); i++ {
		diff := taiTimes[i].Sub(taiTimes[0])
		if math.Abs(float64(diff)) > float64(time.Microsecond) {
			t.Errorf("%s TAI differs from GPS by %v", tests[i].name, diff)
		}
	}
}

// TestTimeTim2TimeGLN verifies GLONASS produces UTCTime (not TAITime).
func TestTimeTim2TimeGLN(t *testing.T) {
	m := parseTimHex(t, tim2CapturedHex["GLN"])
	tm := timeTim2TimeGLN(m)
	if tm.TAITime != 0 {
		t.Errorf("TAITime should be zero for GLONASS, got %v", tm.TAITime)
	}
	if !tm.UTCTime.IsSet() {
		t.Fatal("UTCTime is nil")
	}
	if tm.GNSS != gpsprot.GLO {
		t.Errorf("GNSS = %v, want GLO", tm.GNSS)
	}
	// Captured at 2026-03-22T03:49:56Z
	wantDate := time.Date(2026, 3, 22, 0, 0, 0, 0, time.UTC)
	if tm.UTCTime.Get().Date != wantDate {
		t.Errorf("Date = %v, want %v", tm.UTCTime.Get().Date, wantDate)
	}
	wantTOD := 3*time.Hour + 49*time.Minute + 56*time.Second
	if math.Abs(float64(tm.UTCTime.Get().TimeOfDay-wantTOD)) > float64(time.Millisecond) {
		t.Errorf("TimeOfDay = %v, want ~%v", tm.UTCTime.Get().TimeOfDay, wantTOD)
	}
}

func ptr[T any](v T) opt.Val[T] { return opt.Make(v) }

func TestTimeTim2Tpx(t *testing.T) {
	tests := []struct {
		name string
		hex  string
		want gpsprot.TimeMsg
	}{
		{
			name: "BDS",
			hex:  "bace180012004092d2001efbffff1f046f0001001a00a80704ef210000005f9971f0",
			want: gpsprot.TimeMsg{
				TAITime:     1774151432_999999999,
				Accuracy:    3,
				UTCOffset:   37,
				PulseOffset: ptr(float64(-17) * 0.1),
				GNSS:        gpsprot.BDS,
				Ref:         gpsprot.PostPulse,
				NativeMsgID: "TIM2-TPX",
			},
		},
		{
			name: "invalid",
			hex:  "bace180012000000000000000000000000000000000000000000000000004a005a00",
			want: gpsprot.TimeMsg{Ref: gpsprot.PostPulse, NativeMsgID: "TIM2-TPX"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, err := hex.DecodeString(tt.hex)
			if err != nil {
				t.Fatalf("bad hex: %v", err)
			}
			msg, err := casbin.ParseMsg(string(b))
			if err != nil {
				t.Fatalf("ParseMsg: %v", err)
			}
			tpx, ok := msg.(*casbin.Tim2Tpx)
			if !ok {
				t.Fatalf("unexpected type %T", msg)
			}
			got := timeTim2Tpx(tpx)
			if !reflect.DeepEqual(*got, tt.want) {
				t.Errorf("got  %+v\nwant %+v", *got, tt.want)
			}
		})
	}
}

func TestTimeTim2TimeGNSSInvalid(t *testing.T) {
	m := &casbin.Tim2TimeGNSS{} // TFlag is zero
	tm := timeTim2TimeGNSS(m, gpsprot.GPS, ptime.GPS, ptime.TAIMinusGPS, "TIM2-TIMEGPS")
	if tm == nil {
		t.Fatal("should not return nil")
	}
	if !tm.TAITime.IsZero() {
		t.Errorf("TAITime should be zero, got %v", tm.TAITime)
	}
}

func TestLeapTim2TimeGNSSNoEvent(t *testing.T) {
	m := parseTimHex(t, tim2CapturedHex["GPS"])
	ls := leapTim2TimeGNSS(m, gpsprot.GPS, ptime.TAIMinusGPS)
	if ls != nil {
		t.Errorf("expected nil LeapSecondMsg for NoEvent, got %+v", ls)
	}
}

func TestLeapTim2TimeGNSSEvent(t *testing.T) {
	m := &casbin.Tim2TimeGNSS{
		LsFlag: casbin.Tim2LsEventNormal,
		LsYear: (2026 << 1) | 1, // December 31, 2026
		Ls:     18,
		Lsf:    19,
	}
	ls := leapTim2TimeGNSS(m, gpsprot.GPS, ptime.TAIMinusGPS)
	if ls == nil {
		t.Fatal("expected non-nil LeapSecondMsg")
	}
	if ls.UTCOffBefore != 37 {
		t.Errorf("UTCOffBefore = %d, want 37", ls.UTCOffBefore)
	}
	if ls.UTCOffAfter != 38 {
		t.Errorf("UTCOffAfter = %d, want 38", ls.UTCOffAfter)
	}
	if ls.GNSS != gpsprot.GPS {
		t.Errorf("GNSS = %v, want GPS", ls.GNSS)
	}
	// The event date should be 2027-01-01 00:00:00 TAI (day after Dec 31 + offset)
	wantLS := ptime.LeapSecondOnDate(
		time.Date(2026, time.December, 31, 0, 0, 0, 0, time.UTC),
		37, 38,
	)
	if ls.OffChangeTime != wantLS.OffChangeTime {
		t.Errorf("OffChangeTime = %v, want %v", ls.OffChangeTime, wantLS.OffChangeTime)
	}
}

// leapMsgHandler captures LeapSecondMsg for test verification.
type leapMsgHandler struct {
	gpsprot.DefaultHandler
	times []*gpsprot.TimeMsg
	leaps []*gpsprot.LeapSecondMsg
}

func (h *leapMsgHandler) Time(msg *gpsprot.TimeMsg, _ time.Time) {
	h.times = append(h.times, msg)
}

func (h *leapMsgHandler) LeapSecond(msg *gpsprot.LeapSecondMsg, _ time.Time) {
	h.leaps = append(h.leaps, msg)
}

func TestTim2TimeGPSDispatch(t *testing.T) {
	mgr := gpsprot.NewNavEpochManager()
	pp := NewPacketProcessor(mgr)
	h := &leapMsgHandler{}
	pp.SetMsgHandler(h)
	b, _ := hex.DecodeString(tim2CapturedHex["GPS"])
	_, err := pp.ProcessPacket(string(b), time.Unix(1, 0))
	if err != nil {
		t.Fatalf("ProcessPacket: %v", err)
	}
	if len(h.times) != 1 {
		t.Fatalf("got %d TimeMsgs, want 1", len(h.times))
	}
	tm := h.times[0]
	if tm.Tag != Tag {
		t.Errorf("Tag = %v, want %v", tm.Tag, Tag)
	}
	if tm.TAITime.IsZero() {
		t.Error("TAITime is zero")
	}
	if tm.NativeMsgID != "TIM2-TIMEGPS" {
		t.Errorf("NativeMsgID = %v, want TIM2-TIMEGPS", tm.NativeMsgID)
	}
}

