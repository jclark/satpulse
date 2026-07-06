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
			expect: "setPPSParameters, off",
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
			expect:  "setPPSParameters, , , , , 1",
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

// TestConfigureSignalsCapabilityGate checks that requested signals are
// intersected with the probe's supported list before any command is built:
// unadvertised signals never reach the wire (a refused snt after sst has
// applied would leave partial state), the sst constellation gate reflects
// only constellations with a surviving signal, and the achieved set - the
// snt ack's readback - simply omits what was dropped.
func TestConfigureSignalsCapabilityGate(t *testing.T) {
	sstCmd := "setSatelliteTracking, GPS+GALILEO"
	sntCmd := "setSignalTracking, GALE1BC+GPSL1CA+GALE5"
	snuCmd := "setSignalUsage, GALE1BC+GPSL1CA+GALE5, GALE1BC+GPSL1CA"
	replies := map[string]string{
		"getSignalTracking": "SignalTracking, GPSL1CA+GALE1BC+GALE5",
		"getSignalUsage":    "SignalUsage, GPSL1CA+GALE5, GPSL1CA",
		sstCmd:              "SatelliteTracking, G01+E01",
		sntCmd:              "SignalTracking, GPSL1CA+GALE1BC+GALE5",
		snuCmd:              "SignalUsage, GPSL1CA+GALE1BC+GALE5, GPSL1CA+GALE1BC",
	}
	target := &gpsprot.ConfigTarget{}
	target.Props.SetSignalsEnabled(gpsprot.SignalSetOf(
		gpsprot.SigGPSL1CA, gpsprot.SigGPSL5, gpsprot.SigGALE1, gpsprot.SigGALE6,
		gpsprot.SigNAVICL5))
	cfg, sent, errs := runConfigCaps(t, target, replies, trimmedCapsLine)
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	for _, cmd := range []string{sstCmd, sntCmd, snuCmd} {
		if !slices.Contains(sent, cmd) {
			t.Errorf("command %q not sent; sent: %v", cmd, sent)
		}
	}
	expect := gpsprot.SignalSetOf(gpsprot.SigGPSL1CA, gpsprot.SigGALE1)
	if ss, ok := cfg.ConfigProps().GetSignalsEnabled(); !ok || ss != expect {
		t.Errorf("SignalsEnabled: got %v, %v; want the supported subset %v", ss, ok, expect)
	}
}

// TestSignalUsageExcludesGEO pins the hardware finding that the PVT usage
// argument's enum has no GEO entries: requesting SBAS signals must put
// GEOL1/GEOL5 in the tracking list and the NavData usage list, never in the
// PVT usage list (the receiver refuses it with "Argument 'PVT' is invalid").
func TestSignalUsageExcludesGEO(t *testing.T) {
	c := &Configurator{target: &gpsprot.ConfigTarget{}}
	requested := gpsprot.SignalSetOf(gpsprot.SigGPSL1CA, gpsprot.SigSBASL1CA, gpsprot.SigSBASL5)
	c.caps = &rxCaps{sigSet: requested}
	c.target.Props.SetSignalsEnabled(requested)
	c.np.usage = &signalUsage{pvt: []string{"GPSL1CA", "GLOL2P"}, navData: []string{"GPSL1CA"}}
	c.generateSignalReqs()
	var snu string
	for _, req := range c.reqs {
		if strings.HasPrefix(req.cmd, "setSignalUsage") {
			snu = req.cmd
		}
	}
	want := "setSignalUsage, GPSL1CA+GLOL2P, GEOL1+GEOL5+GPSL1CA"
	if snu != want {
		t.Errorf("got  %q\nwant %q", snu, want)
	}
}
