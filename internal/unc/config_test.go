package unc

import (
	"testing"
	"time"

	"github.com/jclark/satpulse/internal/gpsprot"
)

func TestPPS(t *testing.T) {
	const (
		// Sentinel values to indicate unset properties in test cases
		// These allow us to distinguish between "should be set to zero value" vs "should not be set at all"
		unsetGNSS       = gpsprot.GNSS(0) // Invalid GNSS value (0 is not a valid GNSS constellation)
		unsetCableDelay = -time.Second    // Out of range cable delay (negative values are invalid for cable delay)
	)
	tests := []struct {
		name              string
		timePulse         gpsprot.TimePulse
		antennaCableDelay time.Duration
		timeGNSS          gpsprot.GNSS
		command           string
	}{
		{
			name: "disabled PPS",
			timePulse: gpsprot.TimePulse{
				Width:          0, // Disabled
				Period:         time.Second,
				PolarityRising: true,
				OnlyWhenLocked: true,
				AlignToGNSS:    true,
			},
			antennaCableDelay: unsetCableDelay, // unset for DISABLE
			timeGNSS:          unsetGNSS,       // unset for DISABLE
			command:           "CONFIG PPS DISABLE",
		},
		{
			name: "basic GPS PPS",
			timePulse: gpsprot.TimePulse{
				Width:          time.Millisecond,
				Period:         time.Second,
				PolarityRising: true,
				OnlyWhenLocked: true,
				AlignToGNSS:    true,
			},
			antennaCableDelay: 0,
			timeGNSS:          gpsprot.GPS,
			command:           "CONFIG PPS ENABLE GPS POSITIVE 1000 1000 0 0",
		},
		{
			name: "Galileo PPS with cable delay",
			timePulse: gpsprot.TimePulse{
				Width:          500 * time.Microsecond,
				Period:         time.Second,
				PolarityRising: true,
				OnlyWhenLocked: true,
				AlignToGNSS:    true,
			},
			antennaCableDelay: 50 * time.Nanosecond,
			timeGNSS:          gpsprot.GAL,
			command:           "CONFIG PPS ENABLE GAL POSITIVE 500 1000 50 0",
		},
		{
			name: "BDS negative polarity unlocked",
			timePulse: gpsprot.TimePulse{
				Width:          10 * time.Millisecond,
				Period:         2 * time.Second,
				PolarityRising: false,
				OnlyWhenLocked: false,
				AlignToGNSS:    true,
			},
			antennaCableDelay: -100 * time.Nanosecond,
			timeGNSS:          gpsprot.BDS,
			command:           "CONFIG PPS ENABLE2 BDS NEGATIVE 10000 2000 -100 0",
		},
		{
			name: "manual example",
			timePulse: gpsprot.TimePulse{
				Width:          time.Second / 2,
				Period:         time.Second,
				PolarityRising: true,
				OnlyWhenLocked: true,
				AlignToGNSS:    true,
			},
			antennaCableDelay: 0,
			timeGNSS:          gpsprot.GPS,
			command:           "CONFIG PPS ENABLE GPS POSITIVE 500000 1000 0 0",
		},
		{
			name: "typical example",
			timePulse: gpsprot.TimePulse{
				Width:          time.Second / 10,
				Period:         time.Second,
				PolarityRising: true,
				OnlyWhenLocked: true,
				AlignToGNSS:    true,
			},
			antennaCableDelay: 50,
			timeGNSS:          gpsprot.GPS,
			command:           "CONFIG PPS ENABLE GPS POSITIVE 100000 1000 50 0",
		},
		{
			name: "ENABLE3",
			timePulse: gpsprot.TimePulse{
				Width:          time.Second / 10,
				Period:         time.Second,
				PolarityRising: true,
				OnlyWhenLocked: false,
				AlignToGNSS:    false,
			},
			antennaCableDelay: 0,
			timeGNSS:          gpsprot.GPS,
			command:           "CONFIG PPS ENABLE3 GPS POSITIVE 100000 1000 0 0",
		},
		{
			name: "GLONASS high frequency",
			timePulse: gpsprot.TimePulse{
				Width:          time.Microsecond,
				Period:         100 * time.Millisecond,
				PolarityRising: true,
				OnlyWhenLocked: true,
				AlignToGNSS:    true,
			},
			antennaCableDelay: 32767 * time.Nanosecond, // max int16
			timeGNSS:          gpsprot.GLO,
			command:           "CONFIG PPS ENABLE GLO POSITIVE 1 100 32767 0",
		},
	}

	props := NewNativeConfigProps()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Construct expected ConfigProps using conditional logic once
			var expectedProps gpsprot.ConfigProps
			expectedProps.SetTimePulse(tt.timePulse)
			if tt.timeGNSS != unsetGNSS {
				expectedProps.SetTimeGNSS(tt.timeGNSS)
			}
			if tt.antennaCableDelay != unsetCableDelay {
				expectedProps.SetAntennaCableDelay(tt.antennaCableDelay)
			}

			// Test toNative: gpsprot.ConfigProps -> Unicore command
			pps := props[idPropPPS].newInstance().(*ppsProp)
			err := pps.toNative(&expectedProps)
			if err != nil {
				t.Fatalf("toNative failed: %v", err)
			}

			if pps.command != tt.command {
				t.Errorf("toNative command mismatch:\ngot:  %q\nwant: %q", pps.command, tt.command)
			}

			// Test fromNative: Unicore command -> gpsprot.ConfigProps
			var actualProps gpsprot.ConfigProps
			pps2 := pps.newInstance().(*ppsProp)
			pps2.command = tt.command // Set command to parse
			err = pps2.fromNative(&actualProps)
			if err != nil {
				t.Fatalf("fromNative failed: %v", err)
			}

			// Verify round-trip conversion by comparing actual vs expected
			if actualProps != expectedProps {
				t.Errorf("round-trip mismatch:\ngot:  %+v\nwant: %+v", actualProps, expectedProps)
			}
		})
	}
}
