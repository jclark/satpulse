package timemsg

import (
	"io"
	"log/slog"
	"math"
	"testing"
	"time"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/gpsreg"
	"github.com/jclark/satpulse/gps/lib/opt"
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
		name    string
		entries []struct {
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
				utc := func(h, m, s int, ns int) opt.Val[ptime.UTCTime] {
					return opt.Make(ptime.UTCTime{
						Date:      date,
						TimeOfDay: time.Duration(h)*time.Hour + time.Duration(m)*time.Minute + time.Duration(s)*time.Second + time.Duration(ns),
					})
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
			// Two consecutive NavSolution messages, but n=3: just need to wait.
			name: "insufficient_messages",
			entries: []struct {
				msg   *gpsprot.TimeMsg
				tRead time.Time
			}{
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
						TAITime:     tai(1707882650, 185603),
						GNSS:        gpsprot.GPS,
						Ref:         gpsprot.NavSolution,
						Tag:         gpsreg.TagUBX,
						NativeMsgID: "NAV-TIMEGPS",
					},
					tRead: timeAt(7000832602),
				},
			},
			n:              3,
			expectLast:     0,
			expectTRead:    nil,
			expectMsgLevel: levelSufficient - 1,
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
			n:              3,
			expectLast:     0,
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

			gotLast, gotTRead, _ := buf.GetPostTimeMessages(tt.n)

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
	gotLast, gotTRead, _ := buf.GetPostTimeMessages(3)

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
		PulseOffset: opt.Make(pulseOffset1),
	}, time.Now())

	buf.Time(&gpsprot.TimeMsg{
		TAITime:     tai(101),
		GNSS:        gpsprot.GPS,
		Ref:         gpsprot.PostPulse,
		Tag:         gpsreg.TagUBX,
		NativeMsgID: "TIM-TOS",
		PulseOffset: opt.Make(pulseOffset2),
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
		PulseOffset: opt.Make(pulseOffset),
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
		name     string
		offset   float64
		wantOK   bool
		wantCorr time.Duration
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
				PulseOffset: opt.Make(offset),
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
		PulseOffset: opt.Make(prePulseOffset),
	}, timeAt(99e9+50e6)) // 50ms before second 100

	// PostPulse for second 101 (arrives after pulse)
	buf.Time(&gpsprot.TimeMsg{
		TAITime:     tai(101),
		GNSS:        gpsprot.GPS,
		Ref:         gpsprot.PostPulse,
		Tag:         gpsreg.TagUBX,
		NativeMsgID: "TIM-TOS",
		PulseOffset: opt.Make(postPulseOffset),
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
	gotLast, gotTRead, _ := buf.GetPostTimeMessages(2)

	// Should fail because we only have one PostPulse message (101) and PrePulse is not eligible
	if gotLast != 0 || len(gotTRead) != 0 {
		t.Errorf("GetPostTimeMessages(2) with mixed messages = (%v, len=%d), want (0, len=0) - not enough consecutive messages",
			gotLast, len(gotTRead))
	}
}

type recordingTimer struct {
	samples []msgUTCSample
}

type msgUTCSample struct {
	utc  time.Time
	read time.Time
	leap ptime.LeapSecondKind
}

func (r *recordingTimer) MsgUTCTime(utc time.Time, tRead time.Time, leap ptime.LeapSecondKind) {
	r.samples = append(r.samples, msgUTCSample{utc: utc, read: tRead, leap: leap})
}

func TestMsgUTCTimer(t *testing.T) {
	date := time.Date(2026, 3, 29, 0, 0, 0, 0, time.UTC)
	utc := func(h, m, s int) opt.Val[ptime.UTCTime] {
		return opt.Make(ptime.UTCTime{
			Date:      date,
			TimeOfDay: time.Duration(h)*time.Hour + time.Duration(m)*time.Minute + time.Duration(s)*time.Second,
		})
	}
	ls := ptime.LeapSecond{UTCOffAfter: 37}
	tRead := time.Now()
	readAt := func(ms int) time.Time {
		return tRead.Add(time.Duration(ms) * time.Millisecond)
	}
	type input struct {
		msg   *gpsprot.TimeMsg
		tRead time.Time
	}
	tests := []struct {
		name    string
		msgs    []input
		noSink  bool
		expectN int
	}{
		{
			name:    "eligible_triggers_call",
			msgs:    []input{{&gpsprot.TimeMsg{UTCTime: utc(12, 0, 0)}, readAt(500)}},
			expectN: 1,
		},
		{
			name: "duplicate_second_suppressed",
			msgs: []input{
				{&gpsprot.TimeMsg{UTCTime: utc(12, 0, 0)}, readAt(500)},
				{&gpsprot.TimeMsg{UTCTime: utc(12, 0, 0)}, readAt(600)},
			},
			expectN: 1,
		},
		{
			name: "new_second_triggers",
			msgs: []input{
				{&gpsprot.TimeMsg{UTCTime: utc(12, 0, 0)}, readAt(500)},
				{&gpsprot.TimeMsg{UTCTime: utc(12, 0, 1)}, readAt(1500)},
			},
			expectN: 2,
		},
		{
			name: "tai_only_ignored",
			msgs: func() []input {
				u := utc(12, 0, 0)
				return []input{{&gpsprot.TimeMsg{TAITime: ls.UTCtoTime(u.Get())}, readAt(500)}}
			}(),
			expectN: 0,
		},
		{
			name:    "no_time_skipped",
			msgs:    []input{{&gpsprot.TimeMsg{}, readAt(500)}},
			expectN: 0,
		},
		{
			name:    "leap_second_23_59_60_skipped",
			msgs:    []input{{&gpsprot.TimeMsg{UTCTime: opt.Make(ptime.UTCTime{Date: date, TimeOfDay: 24 * time.Hour})}, readAt(500)}},
			expectN: 0,
		},
		{
			name:    "nil_sink_no_panic",
			msgs:    []input{{&gpsprot.TimeMsg{UTCTime: utc(12, 0, 0)}, readAt(500)}},
			noSink:  true,
			expectN: 0,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			lg := slog.New(slog.NewTextHandler(io.Discard, nil))
			buf := NewBuffer(lg, 5*time.Second, ls, gpsprot.GPS)
			var rec *recordingTimer
			if !tc.noSink {
				rec = &recordingTimer{}
				buf.SetMsgUTCTimer(rec)
			}
			for _, m := range tc.msgs {
				buf.Time(m.msg, m.tRead)
			}
			if rec == nil {
				return
			}
			if len(rec.samples) != tc.expectN {
				t.Errorf("got %d samples, want %d", len(rec.samples), tc.expectN)
			}
		})
	}
}

func TestMsgUTCTimerLeap(t *testing.T) {
	leapLS := ptime.LeapSecondOnDate(time.Date(2026, time.June, 30, 0, 0, 0, 0, time.UTC), 37, 38)
	leapDay := time.Date(2026, time.June, 30, 0, 0, 0, 0, time.UTC)
	normalDay := time.Date(2026, time.March, 29, 0, 0, 0, 0, time.UTC)
	tRead := time.Now()
	tests := []struct {
		name       string
		ut         ptime.UTCTime
		expectLeap ptime.LeapSecondKind
	}{
		{
			name:       "normal_day",
			ut:         ptime.UTCTime{Date: normalDay, TimeOfDay: 15 * time.Hour},
			expectLeap: ptime.LeapSecondNone,
		},
		{
			name:       "leap_day_in_window",
			ut:         ptime.UTCTime{Date: leapDay, TimeOfDay: 20 * time.Hour},
			expectLeap: ptime.LeapSecondPositive,
		},
		{
			name:       "leap_day_before_window",
			ut:         ptime.UTCTime{Date: leapDay, TimeOfDay: 6 * time.Hour},
			expectLeap: ptime.LeapSecondNone,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			lg := slog.New(slog.NewTextHandler(io.Discard, nil))
			buf := NewBuffer(lg, 5*time.Second, leapLS, gpsprot.GPS)
			rec := &recordingTimer{}
			buf.SetMsgUTCTimer(rec)
			buf.Time(&gpsprot.TimeMsg{UTCTime: opt.Make(tc.ut)}, tRead)
			if len(rec.samples) != 1 {
				t.Fatalf("got %d samples, want 1", len(rec.samples))
			}
			if rec.samples[0].leap != tc.expectLeap {
				t.Errorf("leap = %v, want %v", rec.samples[0].leap, tc.expectLeap)
			}
		})
	}
}

func TestMsgUTCTimerReadDelay(t *testing.T) {
	date := time.Date(2026, 3, 29, 0, 0, 0, 0, time.UTC)
	ut := opt.Make(ptime.UTCTime{Date: date, TimeOfDay: 12 * time.Hour})
	ls := ptime.LeapSecond{UTCOffAfter: 37}
	lg := slog.New(slog.NewTextHandler(io.Discard, nil))
	buf := NewBuffer(lg, 5*time.Second, ls, gpsprot.GPS)
	rec := &recordingTimer{}
	buf.SetMsgUTCTimer(rec)
	tRead := time.Now()
	readDelay := 30 * time.Millisecond
	buf.Time(&gpsprot.TimeMsg{UTCTime: ut, ReadDelay: gpsprot.Duration(readDelay)}, tRead)
	if len(rec.samples) != 1 {
		t.Fatalf("got %d samples, want 1", len(rec.samples))
	}
	want := tRead.Add(-readDelay)
	if !rec.samples[0].read.Equal(want) {
		t.Errorf("tRead = %v, want %v (tRead - ReadDelay)", rec.samples[0].read, want)
	}
}

func TestDetectInvalidLeap(t *testing.T) {
	startTime := time.Now()
	timeAt := func(ms int64) time.Time {
		return startTime.Add(time.Duration(ms) * time.Millisecond)
	}
	date := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	utc := func(h, m, s int) opt.Val[ptime.UTCTime] {
		return opt.Make(ptime.UTCTime{
			Date:      date,
			TimeOfDay: time.Duration(h)*time.Hour + time.Duration(m)*time.Minute + time.Duration(s)*time.Second,
		})
	}
	utcOn := func(date time.Time, h, m, s int) opt.Val[ptime.UTCTime] {
		return opt.Make(ptime.UTCTime{
			Date:      date,
			TimeOfDay: time.Duration(h)*time.Hour + time.Duration(m)*time.Minute + time.Duration(s)*time.Second,
		})
	}

	type entry struct {
		msg   *gpsprot.TimeMsg
		tRead time.Time
	}
	rmc := func(u opt.Val[ptime.UTCTime], ms int64) entry {
		return entry{
			msg:   &gpsprot.TimeMsg{UTCTime: u, Tag: gpsreg.TagNMEA, NativeMsgID: "GPRMC"},
			tRead: timeAt(ms),
		}
	}
	gga := func(u opt.Val[ptime.UTCTime], ms int64) entry {
		return entry{
			msg:   &gpsprot.TimeMsg{UTCTime: u, Tag: gpsreg.TagNMEA, NativeMsgID: "GPGGA"},
			tRead: timeAt(ms),
		}
	}

	const minDelta = 800 * time.Millisecond

	tests := []struct {
		name    string
		entries []entry
		expect  bool
	}{
		{
			// Real bug from u-blox LEA-6T coldstart.jsonl: NMEA UTC steps
			// 11:11:08 -> 11:11:06 because firmware was using a stale
			// leap-second count and just learned the correct value from
			// the GPS broadcast.
			name: "lea6t_backwards_2s",
			entries: []entry{
				rmc(utc(11, 11, 5), 3009),
				rmc(utc(11, 11, 6), 3993),
				rmc(utc(11, 11, 7), 5006),
				rmc(utc(11, 11, 8), 6046),
				rmc(utc(11, 11, 6), 7055),
			},
			expect: true,
		},
		{
			name: "backwards_1s",
			entries: []entry{
				rmc(utc(12, 0, 0), 0),
				rmc(utc(11, 59, 59), 1000),
			},
			expect: true,
		},
		{
			name: "duplicate_normal_gap",
			entries: []entry{
				rmc(utc(12, 0, 0), 0),
				rmc(utc(12, 0, 0), 1000),
			},
			expect: true,
		},
		{
			name: "duplicate_short_gap",
			entries: []entry{
				rmc(utc(12, 0, 0), 0),
				rmc(utc(12, 0, 0), 100),
			},
			expect: false,
		},
		{
			// Gap exactly equal to minDelta is treated as a real correction
			// (the suspicious-duplicate test is strict <).
			name: "duplicate_at_threshold",
			entries: []entry{
				rmc(utc(12, 0, 0), 0),
				rmc(utc(12, 0, 0), int64(minDelta/time.Millisecond)),
			},
			expect: true,
		},
		{
			name: "normal_forward",
			entries: []entry{
				rmc(utc(12, 0, 0), 0),
				rmc(utc(12, 0, 1), 1000),
				rmc(utc(12, 0, 2), 2000),
			},
			expect: false,
		},
		{
			// Real positive leap seconds advance civil UTC through :60
			// before the next midnight. The :60 -> next-day-00:00:00
			// step must not be mistaken for a duplicate UTC second.
			name: "real_positive_leap_60_to_next_day",
			entries: []entry{
				rmc(utcOn(time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC), 23, 59, 60), 0),
				rmc(utcOn(time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC), 0, 0, 0), 1000),
			},
			expect: false,
		},
		{
			// Step into the leap second: 23:59:59 -> 23:59:60 on the
			// leap-eligible day. Same-Date pair, but TimeOfDay advances
			// from 23h59m59s to 24h.
			name: "real_positive_leap_into_60",
			entries: []entry{
				rmc(utcOn(time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC), 23, 59, 59), 0),
				rmc(utcOn(time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC), 23, 59, 60), 1000),
			},
			expect: false,
		},
		{
			// Ordinary midnight crossing on a non-leap day: the Date
			// changes between the two entries. Verifies the cross-Date
			// arithmetic on its own, independent of leap-second handling.
			name: "ordinary_midnight_crossing",
			entries: []entry{
				rmc(utcOn(time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC), 23, 59, 59), 0),
				rmc(utcOn(time.Date(2026, 5, 2, 0, 0, 0, 0, time.UTC), 0, 0, 0), 1000),
			},
			expect: false,
		},
		{
			name: "single_message",
			entries: []entry{
				rmc(utc(12, 0, 0), 0),
			},
			expect: false,
		},
		{
			name:    "empty_buffer",
			entries: nil,
			expect:  false,
		},
		{
			// Last entry is GPGGA, not GPRMC; function does not search past
			// the last entry.
			name: "last_not_matching_type",
			entries: []entry{
				rmc(utc(12, 0, 0), 0),
				rmc(utc(11, 59, 59), 1000),
				gga(utc(12, 0, 1), 2000),
			},
			expect: false,
		},
		{
			name: "last_nil_utc",
			entries: []entry{
				rmc(utc(12, 0, 0), 0),
				{
					msg:   &gpsprot.TimeMsg{UTCTime: opt.Val[ptime.UTCTime]{}, Tag: gpsreg.TagNMEA, NativeMsgID: "GPRMC"},
					tRead: timeAt(1000),
				},
			},
			expect: false,
		},
		{
			// Most recent prior matching entry has nil UTCTime; function
			// gives up rather than searching further back.
			name: "prev_nil_utc",
			entries: []entry{
				rmc(utc(12, 0, 0), 0),
				{
					msg:   &gpsprot.TimeMsg{UTCTime: opt.Val[ptime.UTCTime]{}, Tag: gpsreg.TagNMEA, NativeMsgID: "GPRMC"},
					tRead: timeAt(1000),
				},
				rmc(utc(11, 59, 59), 2000),
			},
			expect: false,
		},
		{
			// Backwards leap with intervening other-type messages between
			// the two GPRMC entries; function walks back over them.
			name: "backwards_skips_other_types",
			entries: []entry{
				rmc(utc(12, 0, 0), 0),
				gga(utc(12, 0, 0), 50),
				gga(utc(12, 0, 1), 1050),
				rmc(utc(11, 59, 59), 1000),
			},
			expect: true,
		},
		{
			// Duplicate UTC where raw tRead gap is 1000ms but ReadDelay on
			// the second message corrects its tRead 300ms earlier, putting
			// the corrected gap (700ms) below minDelta.
			name: "duplicate_read_delay_pulls_below_threshold",
			entries: []entry{
				{
					msg:   &gpsprot.TimeMsg{UTCTime: utc(12, 0, 0), Tag: gpsreg.TagNMEA, NativeMsgID: "GPRMC"},
					tRead: timeAt(0),
				},
				{
					msg: &gpsprot.TimeMsg{
						UTCTime:     utc(12, 0, 0),
						Tag:         gpsreg.TagNMEA,
						NativeMsgID: "GPRMC",
						ReadDelay:   gpsprot.Duration(300 * time.Millisecond),
					},
					tRead: timeAt(1000),
				},
			},
			expect: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			lg := slog.New(slog.NewTextHandler(io.Discard, nil))
			buf := NewBuffer(lg, 30*time.Second, ptime.LeapSecond{UTCOffAfter: 37}, gpsprot.GPS)
			for _, e := range tc.entries {
				buf.Time(e.msg, e.tRead)
			}
			got := buf.detectInvalidLeap(gpsreg.TagNMEA, "GPRMC", minDelta)
			if got != tc.expect {
				t.Errorf("detectInvalidLeap() = %v, want %v", got, tc.expect)
			}
		})
	}
}
