package timemsg

import (
	"io"
	"log/slog"
	"math"
	"testing"
	"time"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/gpsreg"
	"github.com/jclark/satpulse/gps/ptime"
)

func TestGetPostTimeMessages(t *testing.T) {
	startTime := time.Now()

	timeAt := func(nanos int64) time.Time {
		return startTime.Add(time.Duration(nanos))
	}

	tai := func(sec int64, ns int64) ptime.Time {
		return ptime.Time(sec*1e9 + ns)
	}

	tests := []struct {
		name       string
		entries    []struct {
			msg   *gpsprot.TimeMsg
			tRead time.Time
		}
		n              int
		expectLast     ptime.Time
		expectTRead    []int64 // expected tRead values as nanos offsets
		expectMsgLevel bufMsgLevel
	}{
		{
			// Real data from u-blox ZED-F9P event log.
			// NAV-TIMEGPS is NavSolution; TIM-TP is PrePulse (ineligible).
			// bestEntries selects NAV-TIMEGPS; times round to consecutive seconds.
			name: "real_data_ubx_navsolution",
			entries: []struct {
				msg   *gpsprot.TimeMsg
				tRead time.Time
			}{
				{
					msg: &gpsprot.TimeMsg{
						TAITime:     tai(1707882643, 186550),
						GNSS:        gpsprot.GPS,
						Ref:         gpsprot.NavSolution,
						Tag:         gpsreg.TagUBX,
						NativeMsgID: "NAV-TIMEGPS",
					},
					tRead: timeAt(0),
				},
				{
					msg: &gpsprot.TimeMsg{
						TAITime:     tai(1707882644, 0),
						GNSS:        gpsprot.GPS,
						Ref:         gpsprot.PrePulse,
						Tag:         gpsreg.TagUBX,
						NativeMsgID: "TIM-TP",
					},
					tRead: timeAt(20106905),
				},
				{
					msg: &gpsprot.TimeMsg{
						TAITime:     tai(1707882644, 186415),
						GNSS:        gpsprot.GPS,
						Ref:         gpsprot.NavSolution,
						Tag:         gpsreg.TagUBX,
						NativeMsgID: "NAV-TIMEGPS",
					},
					tRead: timeAt(1004180411),
				},
				{
					msg: &gpsprot.TimeMsg{
						TAITime:     tai(1707882645, 0),
						GNSS:        gpsprot.GPS,
						Ref:         gpsprot.PrePulse,
						Tag:         gpsreg.TagUBX,
						NativeMsgID: "TIM-TP",
					},
					tRead: timeAt(1030748277),
				},
				{
					msg: &gpsprot.TimeMsg{
						TAITime:     tai(1707882645, 186279),
						GNSS:        gpsprot.GPS,
						Ref:         gpsprot.NavSolution,
						Tag:         gpsreg.TagUBX,
						NativeMsgID: "NAV-TIMEGPS",
					},
					tRead: timeAt(2001227631),
				},
				{
					msg: &gpsprot.TimeMsg{
						TAITime:     tai(1707882646, 0),
						GNSS:        gpsprot.GPS,
						Ref:         gpsprot.PrePulse,
						Tag:         gpsreg.TagUBX,
						NativeMsgID: "TIM-TP",
					},
					tRead: timeAt(2023850023),
				},
				{
					msg: &gpsprot.TimeMsg{
						TAITime:     tai(1707882646, 186144),
						GNSS:        gpsprot.GPS,
						Ref:         gpsprot.NavSolution,
						Tag:         gpsreg.TagUBX,
						NativeMsgID: "NAV-TIMEGPS",
					},
					tRead: timeAt(3004868715),
				},
				{
					msg: &gpsprot.TimeMsg{
						TAITime:     tai(1707882647, 0),
						GNSS:        gpsprot.GPS,
						Ref:         gpsprot.PrePulse,
						Tag:         gpsreg.TagUBX,
						NativeMsgID: "TIM-TP",
					},
					tRead: timeAt(3026795208),
				},
				{
					msg: &gpsprot.TimeMsg{
						TAITime:     tai(1707882647, 186009),
						GNSS:        gpsprot.GPS,
						Ref:         gpsprot.NavSolution,
						Tag:         gpsreg.TagUBX,
						NativeMsgID: "NAV-TIMEGPS",
					},
					tRead: timeAt(4002817020),
				},
				{
					msg: &gpsprot.TimeMsg{
						TAITime:     tai(1707882648, 0),
						GNSS:        gpsprot.GPS,
						Ref:         gpsprot.PrePulse,
						Tag:         gpsreg.TagUBX,
						NativeMsgID: "TIM-TP",
					},
					tRead: timeAt(4022928351),
				},
				{
					msg: &gpsprot.TimeMsg{
						TAITime:     tai(1707882648, 185873),
						GNSS:        gpsprot.GPS,
						Ref:         gpsprot.NavSolution,
						Tag:         gpsreg.TagUBX,
						NativeMsgID: "NAV-TIMEGPS",
					},
					tRead: timeAt(5004023327),
				},
				{
					msg: &gpsprot.TimeMsg{
						TAITime:     tai(1707882649, 0),
						GNSS:        gpsprot.GPS,
						Ref:         gpsprot.PrePulse,
						Tag:         gpsreg.TagUBX,
						NativeMsgID: "TIM-TP",
					},
					tRead: timeAt(5026754453),
				},
				{
					msg: &gpsprot.TimeMsg{
						TAITime:     tai(1707882649, 185738),
						GNSS:        gpsprot.GPS,
						Ref:         gpsprot.NavSolution,
						Tag:         gpsreg.TagUBX,
						NativeMsgID: "NAV-TIMEGPS",
					},
					tRead: timeAt(6005740766),
				},
				{
					msg: &gpsprot.TimeMsg{
						TAITime:     tai(1707882650, 0),
						GNSS:        gpsprot.GPS,
						Ref:         gpsprot.PrePulse,
						Tag:         gpsreg.TagUBX,
						NativeMsgID: "TIM-TP",
					},
					tRead: timeAt(6029888524),
				},
				{
					msg: &gpsprot.TimeMsg{
						TAITime:     tai(1707882650, 185603),
						GNSS:        gpsprot.GPS,
						Ref:         gpsprot.NavSolution,
						Tag:         gpsreg.TagUBX,
						NativeMsgID: "NAV-TIMEGPS",
					},
					tRead: timeAt(7000832602),
				},
				{
					msg: &gpsprot.TimeMsg{
						TAITime:     tai(1707882651, 0),
						GNSS:        gpsprot.GPS,
						Ref:         gpsprot.PrePulse,
						Tag:         gpsreg.TagUBX,
						NativeMsgID: "TIM-TP",
					},
					tRead: timeAt(7017964962),
				},
			},
			n:              3,
			expectLast:     tai(1707882650, 0),
			expectTRead:    []int64{5004023327, 6005740766, 7000832602},
			expectMsgLevel: levelConsecutive,
		},
		{
			// Only PrePulse (TIM-TP) messages: all ineligible for GetPostTimeMessages.
			name: "only_prepulse",
			entries: []struct {
				msg   *gpsprot.TimeMsg
				tRead time.Time
			}{
				{
					msg: &gpsprot.TimeMsg{
						TAITime:     tai(1707882644, 0),
						GNSS:        gpsprot.GPS,
						Ref:         gpsprot.PrePulse,
						Tag:         gpsreg.TagUBX,
						NativeMsgID: "TIM-TP",
					},
					tRead: timeAt(20106905),
				},
				{
					msg: &gpsprot.TimeMsg{
						TAITime:     tai(1707882645, 0),
						GNSS:        gpsprot.GPS,
						Ref:         gpsprot.PrePulse,
						Tag:         gpsreg.TagUBX,
						NativeMsgID: "TIM-TP",
					},
					tRead: timeAt(1030748277),
				},
				{
					msg: &gpsprot.TimeMsg{
						TAITime:     tai(1707882646, 0),
						GNSS:        gpsprot.GPS,
						Ref:         gpsprot.PrePulse,
						Tag:         gpsreg.TagUBX,
						NativeMsgID: "TIM-TP",
					},
					tRead: timeAt(2023850023),
				},
				{
					msg: &gpsprot.TimeMsg{
						TAITime:     tai(1707882647, 0),
						GNSS:        gpsprot.GPS,
						Ref:         gpsprot.PrePulse,
						Tag:         gpsreg.TagUBX,
						NativeMsgID: "TIM-TP",
					},
					tRead: timeAt(3026795208),
				},
				{
					msg: &gpsprot.TimeMsg{
						TAITime:     tai(1707882648, 0),
						GNSS:        gpsprot.GPS,
						Ref:         gpsprot.PrePulse,
						Tag:         gpsreg.TagUBX,
						NativeMsgID: "TIM-TP",
					},
					tRead: timeAt(4022928351),
				},
				{
					msg: &gpsprot.TimeMsg{
						TAITime:     tai(1707882649, 0),
						GNSS:        gpsprot.GPS,
						Ref:         gpsprot.PrePulse,
						Tag:         gpsreg.TagUBX,
						NativeMsgID: "TIM-TP",
					},
					tRead: timeAt(5026754453),
				},
				{
					msg: &gpsprot.TimeMsg{
						TAITime:     tai(1707882650, 0),
						GNSS:        gpsprot.GPS,
						Ref:         gpsprot.PrePulse,
						Tag:         gpsreg.TagUBX,
						NativeMsgID: "TIM-TP",
					},
					tRead: timeAt(6029888524),
				},
				{
					msg: &gpsprot.TimeMsg{
						TAITime:     tai(1707882651, 0),
						GNSS:        gpsprot.GPS,
						Ref:         gpsprot.PrePulse,
						Tag:         gpsreg.TagUBX,
						NativeMsgID: "TIM-TP",
					},
					tRead: timeAt(7017964962),
				},
			},
			n:              3,
			expectLast:     0,
			expectTRead:    nil,
			expectMsgLevel: levelPost - 1,
		},
		{
			// Real data from LBE1421: NMEA GNRMC with .3s offset, not second-aligned.
			name: "nmea_not_second_aligned",
			entries: func() []struct {
				msg   *gpsprot.TimeMsg
				tRead time.Time
			} {
				date := time.Date(2026, 3, 21, 0, 0, 0, 0, time.UTC)
				utc := func(h, m, s int, ns int) *ptime.UTCTime {
					return &ptime.UTCTime{
						Date:      date,
						TimeOfDay: time.Duration(h)*time.Hour + time.Duration(m)*time.Minute + time.Duration(s)*time.Second + time.Duration(ns),
					}
				}
				type e = struct {
					msg   *gpsprot.TimeMsg
					tRead time.Time
				}
				return []e{
					{msg: &gpsprot.TimeMsg{UTCTime: utc(12, 5, 50, 3e8), Tag: gpsreg.TagNMEA, NativeMsgID: "GNRMC"}, tRead: timeAt(0)},
					{msg: &gpsprot.TimeMsg{UTCTime: utc(12, 5, 51, 3e8), Tag: gpsreg.TagNMEA, NativeMsgID: "GNRMC"}, tRead: timeAt(997662796)},
					{msg: &gpsprot.TimeMsg{UTCTime: utc(12, 5, 52, 3e8), Tag: gpsreg.TagNMEA, NativeMsgID: "GNRMC"}, tRead: timeAt(2001736270)},
					{msg: &gpsprot.TimeMsg{UTCTime: utc(12, 5, 53, 3e8), Tag: gpsreg.TagNMEA, NativeMsgID: "GNRMC"}, tRead: timeAt(2998303998)},
					{msg: &gpsprot.TimeMsg{UTCTime: utc(12, 5, 54, 3e8), Tag: gpsreg.TagNMEA, NativeMsgID: "GNRMC"}, tRead: timeAt(3995654589)},
					{msg: &gpsprot.TimeMsg{UTCTime: utc(12, 5, 55, 3e8), Tag: gpsreg.TagNMEA, NativeMsgID: "GNRMC"}, tRead: timeAt(5001573291)},
					{msg: &gpsprot.TimeMsg{UTCTime: utc(12, 5, 56, 3e8), Tag: gpsreg.TagNMEA, NativeMsgID: "GNRMC"}, tRead: timeAt(5999669305)},
					{msg: &gpsprot.TimeMsg{UTCTime: utc(12, 5, 57, 3e8), Tag: gpsreg.TagNMEA, NativeMsgID: "GNRMC"}, tRead: timeAt(6996519195)},
					{msg: &gpsprot.TimeMsg{UTCTime: utc(12, 5, 58, 3e8), Tag: gpsreg.TagNMEA, NativeMsgID: "GNRMC"}, tRead: timeAt(7995389809)},
					{msg: &gpsprot.TimeMsg{UTCTime: utc(12, 5, 59, 3e8), Tag: gpsreg.TagNMEA, NativeMsgID: "GNRMC"}, tRead: timeAt(9000060488)},
				}
			}(),
			n:              3,
			expectLast:     0,
			expectTRead:    nil,
			expectMsgLevel: levelTopOfSecond - 1,
		},
		{
			name: "missing_second_should_fail",
			entries: []struct {
				msg   *gpsprot.TimeMsg
				tRead time.Time
			}{
				{
					msg: &gpsprot.TimeMsg{
						TAITime:     tai(1707442831, 0),
						GNSS:        gpsprot.GPS,
						Ref:         gpsprot.PostPulse,
						Tag:         gpsreg.TagUBX,
						NativeMsgID: "TIM-TP",
					},
					tRead: timeAt(100000000),
				},
				// Missing 1707442832
				{
					msg: &gpsprot.TimeMsg{
						TAITime:     tai(1707442833, 0),
						GNSS:        gpsprot.GPS,
						Ref:         gpsprot.PostPulse,
						Tag:         gpsreg.TagUBX,
						NativeMsgID: "TIM-TP",
					},
					tRead: timeAt(2100000000),
				},
				{
					msg: &gpsprot.TimeMsg{
						TAITime:     tai(1707442834, 0),
						GNSS:        gpsprot.GPS,
						Ref:         gpsprot.PostPulse,
						Tag:         gpsreg.TagUBX,
						NativeMsgID: "TIM-TP",
					},
					tRead: timeAt(3100000000),
				},
			},
			n:          3,
			expectLast: 0,
			expectTRead:    nil, // Should fail due to missing second
			expectMsgLevel: levelConsecutive - 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lg := slog.New(slog.NewTextHandler(io.Discard, nil))
			buf := NewBuffer(lg, 10*time.Second, ptime.LeapSecond{UTCOffAfter: 37}, gpsprot.GPS)

			for _, e := range tt.entries {
				buf.Time(e.msg, e.tRead)
			}

			gotLast, gotTRead := buf.GetPostTimeMessages(tt.n)

			if gotLast != tt.expectLast {
				t.Errorf("GetPostTimeMessages() lastSec = %v, want %v", gotLast, tt.expectLast)
			}

			if len(gotTRead) != len(tt.expectTRead) {
				t.Errorf("GetPostTimeMessages() returned %d times, want %d", len(gotTRead), len(tt.expectTRead))
			}

			if buf.msgLevel != tt.expectMsgLevel {
				t.Errorf("msgLevel = %d, want %d", buf.msgLevel, tt.expectMsgLevel)
			}
			// Check that the tRead values match expected nanos offsets
			for i, tr := range gotTRead {
				if i < len(tt.expectTRead) {
					expectedTime := timeAt(tt.expectTRead[i])
					if !tr.Equal(expectedTime) {
						t.Errorf("tRead[%d] = %v, want %v (nanos: %d)", i, tr, expectedTime, tt.expectTRead[i])
					}
				}
			}
		})
	}
}

func TestPostPulseMessagePreference(t *testing.T) {
	startTime := time.Now()

	timeAt := func(nanos int64) time.Time {
		return startTime.Add(time.Duration(nanos))
	}

	tai := func(sec int64) ptime.Time {
		return ptime.Time(sec * 1e9)
	}

	lg := slog.New(slog.NewTextHandler(io.Discard, nil))
	buf := NewBuffer(lg, 10*time.Second, ptime.LeapSecond{UTCOffAfter: 37}, gpsprot.GPS)

	// Add NavSolution messages for seconds 100, 101, 102
	for i := int64(100); i <= 102; i++ {
		buf.Time(&gpsprot.TimeMsg{
			TAITime:     tai(i),
			GNSS:        gpsprot.GPS,
			Ref:         gpsprot.NavSolution,
			Tag:         gpsreg.TagUBX,
			NativeMsgID: "NAV-PVT",
		}, timeAt(i*1e9+150e6)) // 150ms after second
	}

	// Add PostPulse messages for the same seconds 100, 101, 102
	for i := int64(100); i <= 102; i++ {
		buf.Time(&gpsprot.TimeMsg{
			TAITime:     tai(i),
			GNSS:        gpsprot.GPS,
			Ref:         gpsprot.PostPulse,
			Tag:         gpsreg.TagUBX,
			NativeMsgID: "TIM-TOS",
		}, timeAt(i*1e9+100e6)) // 100ms after second (arrives before NavSolution)
	}

	// Request 3 messages - should get PostPulse messages, not NavSolution
	gotLast, gotTRead := buf.GetPostTimeMessages(3)

	if gotLast != tai(102) {
		t.Errorf("GetPostTimeMessages() lastSec = %v, want %v", gotLast, tai(102))
	}

	if len(gotTRead) != 3 {
		t.Fatalf("GetPostTimeMessages() returned %d times, want 3", len(gotTRead))
	}

	// Verify the tRead values match PostPulse messages (at 100ms), not NavSolution (at 150ms)
	expectedTimes := []int64{100e9 + 100e6, 101e9 + 100e6, 102e9 + 100e6}
	for i, tr := range gotTRead {
		expectedTime := timeAt(expectedTimes[i])
		if !tr.Equal(expectedTime) {
			t.Errorf("tRead[%d] = %v, want %v (PostPulse at 100ms, not NavSolution at 150ms)",
				i, tr, expectedTime)
		}
	}
}

func TestGetPulseCorrectionPostPulse(t *testing.T) {
	lg := slog.New(slog.NewTextHandler(io.Discard, nil))
	buf := NewBuffer(lg, 10*time.Second, ptime.LeapSecond{UTCOffAfter: 37}, gpsprot.GPS)

	tai := func(sec int64) ptime.Time {
		return ptime.Time(sec * 1e9)
	}

	// Add PostPulse messages with sawtooth corrections
	pulseOffset1 := -5.5 // -5.5ns correction
	pulseOffset2 := -3.2 // -3.2ns correction

	buf.Time(&gpsprot.TimeMsg{
		TAITime:     tai(100),
		GNSS:        gpsprot.GPS,
		Ref:         gpsprot.PostPulse,
		Tag:         gpsreg.TagUBX,
		NativeMsgID: "TIM-TOS",
		PulseOffset: &pulseOffset1,
	}, time.Now())

	buf.Time(&gpsprot.TimeMsg{
		TAITime:     tai(101),
		GNSS:        gpsprot.GPS,
		Ref:         gpsprot.PostPulse,
		Tag:         gpsreg.TagUBX,
		NativeMsgID: "TIM-TOS",
		PulseOffset: &pulseOffset2,
	}, time.Now())

	// Test retrieval of PostPulse corrections
	corr, ok := buf.GetPulseCorrection(tai(100))
	if !ok {
		t.Error("GetPulseCorrection(100) returned false, want true")
	}
	if corr != time.Duration(math.Round(pulseOffset1)) {
		t.Errorf("GetPulseCorrection(100) = %v, want %v", corr, time.Duration(math.Round(pulseOffset1)))
	}

	corr, ok = buf.GetPulseCorrection(tai(101))
	if !ok {
		t.Error("GetPulseCorrection(101) returned false, want true")
	}
	if corr != time.Duration(math.Round(pulseOffset2)) {
		t.Errorf("GetPulseCorrection(101) = %v, want %v", corr, time.Duration(math.Round(pulseOffset2)))
	}

	// Test that correction not available for future time
	_, ok = buf.GetPulseCorrection(tai(102))
	if ok {
		t.Error("GetPulseCorrection(102) returned true, want false (not yet available)")
	}
}

func TestWaitForPulseCorrectionPostPulse(t *testing.T) {
	lg := slog.New(slog.NewTextHandler(io.Discard, nil))
	buf := NewBuffer(lg, 10*time.Second, ptime.LeapSecond{UTCOffAfter: 37}, gpsprot.GPS)

	tai := func(sec int64) ptime.Time {
		return ptime.Time(sec * 1e9)
	}

	pulseOffset := -4.0
	buf.Time(&gpsprot.TimeMsg{
		TAITime:     tai(100),
		GNSS:        gpsprot.GPS,
		Ref:         gpsprot.PostPulse,
		Tag:         gpsreg.TagUBX,
		NativeMsgID: "TIM-TOS",
		PulseOffset: &pulseOffset,
	}, time.Now())

	// WaitForPulseCorrection should return true for the next second (101)
	// when we have a PostPulse message (not PrePulse)
	if !buf.WaitForPulseCorrection(tai(101)) {
		t.Error("WaitForPulseCorrection(101) = false, want true (PostPulse message expected)")
	}

	// Should return false for seconds other than the next one
	if buf.WaitForPulseCorrection(tai(102)) {
		t.Error("WaitForPulseCorrection(102) = true, want false (too far in future)")
	}

	if buf.WaitForPulseCorrection(tai(100)) {
		t.Error("WaitForPulseCorrection(100) = true, want false (already have correction)")
	}
}

func TestValidatePulseOffset(t *testing.T) {
	lg := slog.New(slog.NewTextHandler(io.Discard, nil))
	buf := NewBuffer(lg, 10*time.Second, ptime.LeapSecond{UTCOffAfter: 37}, gpsprot.GPS)

	tai := func(sec int64) ptime.Time {
		return ptime.Time(sec * 1e9)
	}

	tests := []struct {
		name       string
		offset     float64
		wantOK     bool
		wantCorr   time.Duration
	}{
		{"valid_small_positive", 5.5, true, 6},
		{"valid_small_negative", -5.5, true, -6},
		{"valid_at_limit", 100.0, true, 100},
		{"valid_at_negative_limit", -100.0, true, -100},
		{"invalid_exceeds_limit", 100.1, false, 0},
		{"invalid_exceeds_negative_limit", -100.1, false, 0},
		{"invalid_bogus_microseconds", 100000.0, false, 0}, // 100µs bogus value from issue #166
		{"valid_zero", 0.0, true, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			offset := tt.offset
			buf.Time(&gpsprot.TimeMsg{
				TAITime:     tai(200),
				GNSS:        gpsprot.GPS,
				Ref:         gpsprot.PostPulse,
				Tag:         gpsreg.TagUBX,
				NativeMsgID: "TIM-TOS",
				PulseOffset: &offset,
			}, time.Now())

			corr, ok := buf.GetPulseCorrection(tai(200))
			if ok != tt.wantOK {
				t.Errorf("GetPulseCorrection() ok = %v, want %v", ok, tt.wantOK)
			}
			if ok && corr != tt.wantCorr {
				t.Errorf("GetPulseCorrection() corr = %v, want %v", corr, tt.wantCorr)
			}
		})
	}
}

func TestMixedPrePulseAndPostPulse(t *testing.T) {
	startTime := time.Now()

	timeAt := func(nanos int64) time.Time {
		return startTime.Add(time.Duration(nanos))
	}

	tai := func(sec int64) ptime.Time {
		return ptime.Time(sec * 1e9)
	}

	lg := slog.New(slog.NewTextHandler(io.Discard, nil))
	buf := NewBuffer(lg, 10*time.Second, ptime.LeapSecond{UTCOffAfter: 37}, gpsprot.GPS)

	// Mix of PrePulse and PostPulse messages
	prePulseOffset := -6.0
	postPulseOffset := -4.5

	// PrePulse for second 100 (arrives before pulse)
	buf.Time(&gpsprot.TimeMsg{
		TAITime:     tai(100),
		GNSS:        gpsprot.GPS,
		Ref:         gpsprot.PrePulse,
		Tag:         gpsreg.TagUBX,
		NativeMsgID: "TIM-TP",
		PulseOffset: &prePulseOffset,
	}, timeAt(99e9+50e6)) // 50ms before second 100

	// PostPulse for second 101 (arrives after pulse)
	buf.Time(&gpsprot.TimeMsg{
		TAITime:     tai(101),
		GNSS:        gpsprot.GPS,
		Ref:         gpsprot.PostPulse,
		Tag:         gpsreg.TagUBX,
		NativeMsgID: "TIM-TOS",
		PulseOffset: &postPulseOffset,
	}, timeAt(101e9+100e6)) // 100ms after second 101

	// NavSolution messages for both seconds (should be ignored in favor of PrePulse/PostPulse)
	buf.Time(&gpsprot.TimeMsg{
		TAITime:     tai(100),
		GNSS:        gpsprot.GPS,
		Ref:         gpsprot.NavSolution,
		Tag:         gpsreg.TagUBX,
		NativeMsgID: "NAV-PVT",
	}, timeAt(100e9+150e6))

	buf.Time(&gpsprot.TimeMsg{
		TAITime:     tai(101),
		GNSS:        gpsprot.GPS,
		Ref:         gpsprot.NavSolution,
		Tag:         gpsreg.TagUBX,
		NativeMsgID: "NAV-PVT",
	}, timeAt(101e9+150e6))

	// Verify GetPulseCorrection retrieves both types correctly
	corr, ok := buf.GetPulseCorrection(tai(100))
	if !ok || corr != time.Duration(math.Round(prePulseOffset)) {
		t.Errorf("GetPulseCorrection(100) = (%v, %v), want (%v, true) from PrePulse",
			corr, ok, time.Duration(math.Round(prePulseOffset)))
	}

	corr, ok = buf.GetPulseCorrection(tai(101))
	if !ok || corr != time.Duration(math.Round(postPulseOffset)) {
		t.Errorf("GetPulseCorrection(101) = (%v, %v), want (%v, true) from PostPulse",
			corr, ok, time.Duration(math.Round(postPulseOffset)))
	}

	// GetPostTimeMessages should prefer PostPulse for 101 (but there's only one PostPulse message)
	// So we can't get 2 consecutive messages - this verifies filtering works
	gotLast, gotTRead := buf.GetPostTimeMessages(2)

	// Should fail because we only have one PostPulse message (101) and PrePulse is not eligible
	if gotLast != 0 || len(gotTRead) != 0 {
		t.Errorf("GetPostTimeMessages(2) with mixed messages = (%v, len=%d), want (0, len=0) - not enough consecutive messages",
			gotLast, len(gotTRead))
	}
}