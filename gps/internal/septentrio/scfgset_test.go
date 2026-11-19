package septentrio

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jclark/satpulse/gps/gpsprot"
)

func TestNearestPPSInterval(t *testing.T) {
	tests := []struct {
		name   string
		d      time.Duration
		expect string
	}{
		{name: "exact second", d: time.Second, expect: "sec1"},
		{name: "rounds to nearest", d: 900 * time.Millisecond, expect: "sec1"},
		{name: "tie prefers shorter period", d: 3 * time.Second, expect: "sec2"},
		{name: "exact 100ms", d: 100 * time.Millisecond, expect: "msec100"},
		{name: "above range clamps to longest", d: 2 * time.Hour, expect: "sec60"},
		{name: "below range clamps to shortest", d: time.Nanosecond, expect: "MHz10"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := nearestPPSInterval(tc.d); got != tc.expect {
				t.Errorf("got %q want %q", got, tc.expect)
			}
		})
	}
}

func TestPPSSetCmd(t *testing.T) {
	setPulse := func(props *gpsprot.ConfigProps, tp gpsprot.TimePulse) *gpsprot.ConfigProps {
		props.SetTimePulse(tp)
		return props
	}
	tests := []struct {
		name    string
		props   func(*gpsprot.ConfigProps)
		current *ppsParams // queried state, nil if none
		expect  string
	}{
		{
			name:   "no time pulse properties",
			props:  func(p *gpsprot.ConfigProps) { p.SetMinElevation(5 * gpsprot.Degrees) },
			expect: "",
		},
		{
			name:   "zero width disables",
			props:  func(p *gpsprot.ConfigProps) { setPulse(p, gpsprot.TimePulse{Width: 0, Period: time.Second}) },
			expect: "setPPSParameters, off, High2Low, , RxClock, 0",
		},
		{
			name: "zero width keeps a simultaneous TimeGNSS",
			props: func(p *gpsprot.ConfigProps) {
				setPulse(p, gpsprot.TimePulse{Width: 0, Period: time.Second})
				p.SetTimeGNSS(gpsprot.GAL)
			},
			current: &ppsParams{interval: "sec1", timeScale: "GPS"},
			expect:  "setPPSParameters, off, High2Low, , Galileo, 0",
		},
		{
			name: "full pulse with TimeGNSS",
			props: func(p *gpsprot.ConfigProps) {
				setPulse(p, gpsprot.TimePulse{Width: 200 * time.Microsecond, Period: 100 * time.Millisecond,
					AlignToGNSS: true, OnlyWhenLocked: false, PolarityRising: false})
				p.SetTimeGNSS(gpsprot.GAL)
			},
			current: &ppsParams{interval: "sec1", timeScale: "GPS"},
			expect:  "setPPSParameters, msec100, High2Low, , Galileo, 0, 0.2",
		},
		{
			name: "width alone enables a disabled pulse with the default interval",
			props: func(p *gpsprot.ConfigProps) {
				p.SetTimePulseWidth(5 * time.Millisecond)
			},
			current: &ppsParams{interval: "off", timeScale: "GPS"},
			expect:  "setPPSParameters, sec1, , , , , 5",
		},
		{
			name: "align to GNSS false selects RxClock",
			props: func(p *gpsprot.ConfigProps) {
				p.SetTimePulseAlignToGNSS(false)
			},
			current: &ppsParams{interval: "sec1", timeScale: "GPS"},
			expect:  "setPPSParameters, , , , RxClock",
		},
		{
			name: "align to GNSS true leaves an aligned time scale alone",
			props: func(p *gpsprot.ConfigProps) {
				p.SetTimePulseAlignToGNSS(true)
				p.SetTimePulseOnlyWhenLocked(true)
			},
			current: &ppsParams{interval: "sec1", timeScale: "GPS"},
			expect:  "setPPSParameters, , , , , 60",
		},
		{
			name: "only when locked keeps an existing holdover",
			props: func(p *gpsprot.ConfigProps) {
				p.SetTimePulseOnlyWhenLocked(true)
			},
			current: &ppsParams{interval: "sec1", timeScale: "GPS", maxHoldover: 30},
			expect:  "",
		},
		{
			name: "align to GNSS true moves RxClock to GPS",
			props: func(p *gpsprot.ConfigProps) {
				p.SetTimePulseAlignToGNSS(true)
			},
			current: &ppsParams{interval: "sec1", timeScale: "RxClock"},
			expect:  "setPPSParameters, , , , GPS",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := &Configurator{target: &gpsprot.ConfigTarget{}}
			tc.props(&c.target.Props)
			c.np.pps = tc.current
			if got := c.ppsSetCmd(); got != tc.expect {
				t.Errorf("got  %q\nwant %q", got, tc.expect)
			}
		})
	}
}

// TestMinElevationCmd checks the elevation-mask range and rounding: the
// receiver accepts whole degrees in -90..90, so negative masks pass through,
// fractions round up (never admitting an excluded satellite), and only
// out-of-range values clamp.
func TestMinElevationCmd(t *testing.T) {
	tests := []struct {
		v      gpsprot.Angle
		expect string
	}{
		{-5 * gpsprot.Degrees, "setElevationMask, PVT, -5"},
		{-gpsprot.Degrees * 3 / 2, "setElevationMask, PVT, -1"},
		{gpsprot.DegreesFromFloat(5.2), "setElevationMask, PVT, 6"},
		{-gpsprot.Degrees / 2, "setElevationMask, PVT, 0"},
		{-100 * gpsprot.Degrees, "setElevationMask, PVT, -90"},
		{100 * gpsprot.Degrees, "setElevationMask, PVT, 90"},
	}
	for _, tc := range tests {
		c := &Configurator{target: &gpsprot.ConfigTarget{}}
		c.target.Props.SetMinElevation(tc.v)
		c.generateScalarReqs()
		if len(c.reqs) != 1 || c.reqs[0].cmd != tc.expect {
			t.Errorf("MinElevation %v: got %v want %q", tc.v, c.reqs, tc.expect)
		}
	}
}

// TestConfigureSignalsCapabilityGate checks that requested signals are
// intersected with the probe's supported list before any command is built:
// unadvertised signals never reach the wire (a refused snt after sst has
// applied would leave partial state), the sst constellation gate reflects
// only constellations with a surviving signal, and the achieved set - the
// snt ack's readback - simply omits what was dropped.
func TestConfigureSignalsCapabilityGate(t *testing.T) {
	sstCmd := "setSatelliteTracking, GPS+GALILEO"
	sntCmd := "setSignalTracking, GALE1BC+GPSL1CA+GALE5"
	replies := map[string]string{
		"getSatelliteTracking": allSatsTracking,
		"getSignalTracking":    "SignalTracking, GPSL1CA+GLOL1CA+GALE1BC+GALE5",
		sstCmd:                 "SatelliteTracking, G01+E01",
		sntCmd:                 "SignalTracking, GPSL1CA+GALE1BC+GALE5",
	}
	target := &gpsprot.ConfigTarget{}
	target.Props.SetSignalsEnabled(gpsprot.SignalSetOf(
		gpsprot.SigGPSL1CA, gpsprot.SigGPSL5, gpsprot.SigGALE1, gpsprot.SigGALE6,
		gpsprot.SigNAVICL5))
	cfg, sent, errs := runConfigCaps(t, target, replies, trimmedCapsLine)
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	for _, cmd := range []string{sstCmd, sntCmd} {
		if !slices.Contains(sent, cmd) {
			t.Errorf("command %q not sent; sent: %v", cmd, sent)
		}
	}
	if slices.ContainsFunc(sent, func(cmd string) bool { return strings.Contains(cmd, "SignalUsage") }) {
		t.Errorf("signal usage touched; sent: %v", sent)
	}
	expect := gpsprot.SignalSetOf(gpsprot.SigGPSL1CA, gpsprot.SigGALE1)
	if ss, ok := cfg.ConfigProps().GetSignalsEnabled(); !ok || ss != expect {
		t.Errorf("SignalsEnabled: got %v, %v; want the supported subset %v", ss, ok, expect)
	}
}

// TestConfigureSignalsUnchanged checks that a tracking command that would
// leave the queried state as it is is not sent: setSignalTracking restarts
// all tracking loops.
func TestConfigureSignalsUnchanged(t *testing.T) {
	sstCmd := "setSatelliteTracking, GPS+GALILEO"
	sntCmd := "setSignalTracking, GALE1BC+GPSL1CA+GALE5"
	tests := []struct {
		name     string
		sats     string
		tracking string
		expect   []string
	}{
		{
			name:     "both unchanged",
			sats:     "SatelliteTracking, G01+G02+E01",
			tracking: "SignalTracking, GPSL1CA+GALE1BC+GALE5",
			expect:   nil,
		},
		{
			name:     "constellations differ",
			sats:     allSatsTracking,
			tracking: "SignalTracking, GPSL1CA+GALE1BC+GALE5",
			expect:   []string{sstCmd},
		},
		{
			name:     "signals differ",
			sats:     "SatelliteTracking, G01+G02+E01",
			tracking: "SignalTracking, GPSL1CA+GPSL2C+GALE1BC+GALE5",
			expect:   []string{sntCmd},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			replies := map[string]string{
				"getSatelliteTracking": tc.sats,
				"getSignalTracking":    tc.tracking,
				sstCmd:                 "SatelliteTracking, G01+G02+E01",
				sntCmd:                 "SignalTracking, GPSL1CA+GALE1BC+GALE5",
			}
			target := &gpsprot.ConfigTarget{}
			target.Props.SetSignalsEnabled(gpsprot.SignalSetOf(gpsprot.SigGPSL1CA, gpsprot.SigGALE1))
			cfg, sent, errs := runConfig(t, target, replies)
			if len(errs) > 0 {
				t.Fatalf("unexpected errors: %v", errs)
			}
			var got []string
			for _, cmd := range sent {
				if strings.HasPrefix(cmd, "set") {
					got = append(got, cmd)
				}
			}
			if !slices.Equal(got, tc.expect) {
				t.Errorf("set commands: got %v, want %v", got, tc.expect)
			}
			expect := gpsprot.SignalSetOf(gpsprot.SigGPSL1CA, gpsprot.SigGALE1)
			if ss, ok := cfg.ConfigProps().GetSignalsEnabled(); !ok || ss != expect {
				t.Errorf("SignalsEnabled: got %v, %v; want %v", ss, ok, expect)
			}
		})
	}
}

// TestConfigureSignalsEmpty checks that a request leaving no signal to enable
// is realized by the constellation gate alone: setSignalTracking has no
// empty-list value.
func TestConfigureSignalsEmpty(t *testing.T) {
	sstCmd := "setSatelliteTracking, none"
	replies := map[string]string{
		"getSatelliteTracking": allSatsTracking,
		"getSignalTracking":    "SignalTracking, GPSL1CA+GALE1BC",
		sstCmd:                 "SatelliteTracking, none",
	}
	target := &gpsprot.ConfigTarget{}
	target.Props.SetSignalsEnabled(gpsprot.SignalSetOf(gpsprot.SigNAVICL5))
	cfg, sent, errs := runConfigCaps(t, target, replies, trimmedCapsLine)
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	var got []string
	for _, cmd := range sent {
		if strings.HasPrefix(cmd, "set") {
			got = append(got, cmd)
		}
	}
	if expect := []string{sstCmd}; !slices.Equal(got, expect) {
		t.Errorf("set commands: got %v, want %v", got, expect)
	}
	if ss, ok := cfg.ConfigProps().GetSignalsEnabled(); !ok || ss != 0 {
		t.Errorf("SignalsEnabled: got %v, %v; want none", ss, ok)
	}
}
