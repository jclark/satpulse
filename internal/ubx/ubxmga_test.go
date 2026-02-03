package ubx

import (
	"testing"
	"time"

	"github.com/jclark/satpulse/internal/gpsprot"
	"github.com/jclark/satpulse/internal/ptime"
	"github.com/jclark/satpulse/internal/ubxbin"
)

func TestMgaTime(t *testing.T) {
	tests := []struct {
		name     string
		ta       *gpsprot.TimeEstimate
		now      time.Time
		wantNil  bool
		wantErr  bool
		validate func(*testing.T, *ubxbin.MgaIni)
	}{
		{
			name: "nil time estimate",
			ta: &gpsprot.TimeEstimate{
				EstimatedTime: time.Time{}, // zero time
			},
			now:     time.Now(),
			wantNil: true,
		},
		{
			name: "basic time estimate",
			ta: &gpsprot.TimeEstimate{
				EstimatedTime: time.Date(2024, 12, 15, 10, 30, 45, 123456789, time.UTC),
				Accuracy:      time.Second * 5,
				Trusted:       true,
				LeapSecond: ptime.LeapSecondState{
					UTCOffset:   37, // Current TAI-UTC offset
					LeapTonight: ptime.LeapSecondNone,
				},
			},
			now: time.Date(2024, 12, 15, 10, 30, 45, 0, time.UTC),
			validate: func(t *testing.T, msg *ubxbin.MgaIni) {
				if msg.Type != ubxbin.MgaIniTypeTimeUTC {
					t.Errorf("expected Type=%d, got %d", ubxbin.MgaIniTypeTimeUTC, msg.Type)
				}
				if msg.Version != ubxbin.MgaIniVersion {
					t.Errorf("expected Version=%d, got %d", ubxbin.MgaIniVersion, msg.Version)
				}
				utc, ok := msg.Payload.(*ubxbin.MgaIniTimeUTC)
				if !ok {
					t.Fatalf("expected *MgaIniTimeUTC, got %T", msg.Payload)
				}
				if utc.Year != 2024 {
					t.Errorf("expected Year=2024, got %d", utc.Year)
				}
				if utc.Month != 12 {
					t.Errorf("expected Month=12, got %d", utc.Month)
				}
				if utc.Day != 15 {
					t.Errorf("expected Day=15, got %d", utc.Day)
				}
				if utc.Hour != 10 {
					t.Errorf("expected Hour=10, got %d", utc.Hour)
				}
				if utc.Minute != 30 {
					t.Errorf("expected Minute=30, got %d", utc.Minute)
				}
				if utc.Second != 45 {
					t.Errorf("expected Second=45, got %d", utc.Second)
				}
				if utc.Ns != 123456789 {
					t.Errorf("expected Ns=123456789, got %d", utc.Ns)
				}
				if utc.TAccS != 5 {
					t.Errorf("expected TAccS=5, got %d", utc.TAccS)
				}
				if utc.TAccNs != 100000000 { // 100ms added for transmission latency
					t.Errorf("expected TAccNs=100000000, got %d", utc.TAccNs)
				}
				expectedLeapSecs := int8(37 - ptime.TAIMinusGPS) // 37 - 19 = 18
				if utc.LeapSecs != expectedLeapSecs {
					t.Errorf("expected LeapSecs=%d, got %d", expectedLeapSecs, utc.LeapSecs)
				}
				if utc.Bitfield0&ubxbin.MgaIniTimeTrustedSource == 0 {
					t.Error("expected TrustedSource bit to be set")
				}
				if utc.Ref != ubxbin.MgaIniTimeRefSourceNone {
					t.Errorf("expected Ref=%d, got %d", ubxbin.MgaIniTimeRefSourceNone, utc.Ref)
				}
			},
		},
		{
			name: "default accuracy",
			ta: &gpsprot.TimeEstimate{
				EstimatedTime: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
				Accuracy:      0, // use default accuracy
			},
			now: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			validate: func(t *testing.T, msg *ubxbin.MgaIni) {
				utc := msg.Payload.(*ubxbin.MgaIniTimeUTC)
				if utc.TAccS != 10 { // default 10 seconds
					t.Errorf("expected TAccS=10, got %d", utc.TAccS)
				}
				if utc.TAccNs != 0 {
					t.Errorf("expected TAccNs=0, got %d", utc.TAccNs)
				}
			},
		},
		{
			name: "time adjustment with estimate time",
			ta: &gpsprot.TimeEstimate{
				EstimatedTime:  time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
				TimeOfEstimate: time.Date(2024, 1, 1, 11, 59, 0, 0, time.UTC), // 1 minute ago
				Accuracy:       time.Second * 2,
			},
			now: time.Date(2024, 1, 1, 12, 1, 0, 0, time.UTC), // 2 minutes after estimate time
			validate: func(t *testing.T, msg *ubxbin.MgaIni) {
				utc := msg.Payload.(*ubxbin.MgaIniTimeUTC)
				// Should be adjusted by 2 minutes forward from original estimate
				if utc.Hour != 12 || utc.Minute != 2 {
					t.Errorf("expected time 12:02, got %02d:%02d", utc.Hour, utc.Minute)
				}
			},
		},
		{
			name: "unknown leap seconds",
			ta: &gpsprot.TimeEstimate{
				EstimatedTime: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
				LeapSecond: ptime.LeapSecondState{
					UTCOffset:   0, // unknown
					LeapTonight: ptime.LeapSecondNone,
				},
			},
			now: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			validate: func(t *testing.T, msg *ubxbin.MgaIni) {
				utc := msg.Payload.(*ubxbin.MgaIniTimeUTC)
				if utc.LeapSecs != ubxbin.MgaIniTimeUTCLeapSecsUnknown {
					t.Errorf("expected LeapSecs=%d, got %d", ubxbin.MgaIniTimeUTCLeapSecsUnknown, utc.LeapSecs)
				}
			},
		},
		{
			name: "untrusted source",
			ta: &gpsprot.TimeEstimate{
				EstimatedTime: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
				Trusted:       false,
			},
			now: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			validate: func(t *testing.T, msg *ubxbin.MgaIni) {
				utc := msg.Payload.(*ubxbin.MgaIniTimeUTC)
				if utc.Bitfield0&ubxbin.MgaIniTimeTrustedSource != 0 {
					t.Error("expected TrustedSource bit to be unset")
				}
			},
		},
		{
			name: "very large accuracy error",
			ta: &gpsprot.TimeEstimate{
				EstimatedTime: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
				Accuracy:      time.Hour * 24 * 365, // very large accuracy > MaxUint16 seconds
			},
			now:     time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			wantErr: true,
		},
		{
			name: "leap second crossing avoidance",
			ta: &gpsprot.TimeEstimate{
				EstimatedTime:  time.Date(2024, 6, 30, 23, 59, 50, 0, time.UTC), // 10 seconds before potential leap
				TimeOfEstimate: time.Date(2024, 6, 30, 23, 59, 45, 0, time.UTC), // 5 seconds before estimate
				LeapSecond: ptime.LeapSecondState{
					UTCOffset:   37,
					LeapTonight: ptime.LeapSecondPositive, // leap second tonight
				},
			},
			now: time.Date(2024, 7, 1, 0, 0, 10, 0, time.UTC), // 10 seconds after midnight (crossed leap)
			validate: func(t *testing.T, msg *ubxbin.MgaIni) {
				utc := msg.Payload.(*ubxbin.MgaIniTimeUTC)
				// Should give up on leap second calculation and set unknown
				if utc.LeapSecs != ubxbin.MgaIniTimeUTCLeapSecsUnknown {
					t.Errorf("expected LeapSecs=%d for leap crossing, got %d", ubxbin.MgaIniTimeUTCLeapSecsUnknown, utc.LeapSecs)
				}
				// Accuracy should be increased by 1 second
				if utc.TAccS < 11 { // default 10 + 1 for uncertainty
					t.Errorf("expected TAccS >= 11 for leap crossing uncertainty, got %d", utc.TAccS)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := mgaTime(tt.ta, tt.now)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tt.wantNil {
				if result != nil {
					t.Error("expected nil result, got non-nil")
				}
				return
			}

			if result == nil {
				t.Fatal("expected non-nil result, got nil")
			}

			if tt.validate != nil {
				tt.validate(t, result)
			}
		})
	}
}
