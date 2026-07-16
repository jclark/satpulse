package gpscmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jclark/satpulse/gps/gpsprot"
)

func TestGnssListSet(t *testing.T) {
	gl := &gnssList{}
	err := gl.Set("GPS,GLO")
	if err != nil {
		t.Errorf("Set returned error: %v", err)
	}

	if len(gl.gnss) != 2 {
		t.Errorf("Expected length of 2, got %v", len(gl.gnss))
	}

	if gl.gnss[0] != gpsprot.GPS {
		t.Errorf("Expected first element to be GPS, got %v", gl.gnss[0])
	}

	if gl.gnss[1] != gpsprot.GLO {
		t.Errorf("Expected second element to be GLO, got %v", gl.gnss[1])
	}

	err = gl.Set("")
	if err == nil {
		t.Errorf("Expected error, got nil")
	}
	if err.Error() != "invalid GNSS name: empty string" {
		t.Errorf("Unexpected error message for empty string, got %v", err.Error())
	}
}

func TestCreateConfigTargetProbeOnly(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want bool
	}{
		{
			name: "serial device with speed",
			args: []string{"-d", "/dev/ttyACM0", "-s", "9600"},
			want: true,
		},
		{
			name: "socket connection",
			args: []string{"--socket", "/tmp/gps.sock"},
			want: true,
		},
		{
			name: "serial device with configuration change",
			args: []string{"-d", "/dev/ttyACM0", "-s", "9600", "--gnss", "GPS"},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flagVars, _, err := parseFlags("gps", tt.args)
			if err != nil {
				t.Fatalf("parseFlags failed: %v", err)
			}
			if flagVars == nil {
				t.Fatalf("parseFlags returned nil flagVars")
			}

			target, err := createConfigTarget(flagVars)
			if err != nil {
				t.Fatalf("createConfigTarget failed: %v", err)
			}

			got := configTargetIsProbeOnly(target)
			if got != tt.want {
				t.Errorf("configTargetIsProbeOnly() = %v, want %v", got, tt.want)
				t.Logf("target.Get = %v", target.Get)
				t.Logf("target.Props.IsEmpty() = %v", target.Props.IsEmpty())
				t.Logf("target.Opts.NoOp() = %v", target.Opts.NoOp())
				t.Logf("target.Opts.Socket = %v", target.Opts.Socket)
				t.Logf("target.Opts.ForceProbe = %v", target.Opts.ForceProbe)
			}
		})
	}
}

func TestCreateConfigTargetJSON(t *testing.T) {
	v := &flagVars{
		targetJSON: `{"Props":{"mode":{"static":true}},"Get":["baudRate"],"Opts":{"Save":"minimal","NMEAMsg":[]}}`,
	}
	target, err := createConfigTarget(v)
	if err != nil {
		t.Fatalf("createConfigTarget: %v", err)
	}
	mode, ok := target.Props.GetMode()
	if !ok || !mode.Static {
		t.Errorf("mode = %+v, %t, want static mode", mode, ok)
	}
	if target.Get != gpsprot.PropIDBaudRate {
		t.Errorf("Get = %v, want baudRate", target.Get)
	}
	if target.Opts.Save != gpsprot.SaveMinimal {
		t.Errorf("Save = %v, want SaveMinimal", target.Opts.Save)
	}
	if !target.Opts.NMEAMsg.IsSet() || target.Opts.NMEAMsg.Get() != gpsprot.NMEAMsgNone {
		t.Errorf("NMEAMsg = %+v, want set-empty", target.Opts.NMEAMsg)
	}
}

// Opts.Socket describes the transport, so the JSON must not be able to claim a
// proxy connection on a serial device: that would skip the silence wait, the
// detection deadline, and the framing checks in gpscfg.Configure.
func TestCreateConfigTargetJSONSocketFollowsTransport(t *testing.T) {
	target, err := createConfigTarget(&flagVars{targetJSON: `{"Opts":{"Socket":true}}`, serialDevice: "/dev/ttyACM0"})
	if err != nil {
		t.Fatalf("createConfigTarget: %v", err)
	}
	if target.Opts.Socket {
		t.Error("Socket = true for a serial transport")
	}
	target, err = createConfigTarget(&flagVars{targetJSON: `{}`, socketPath: "/tmp/socket"})
	if err != nil {
		t.Fatalf("createConfigTarget: %v", err)
	}
	if !target.Opts.Socket {
		t.Error("Socket = false for a socket transport")
	}
}

func TestCreateConfigTargetJSONNoOp(t *testing.T) {
	target, err := createConfigTarget(&flagVars{targetJSON: `{}`})
	if err != nil {
		t.Fatalf("createConfigTarget: %v", err)
	}
	if target == nil || !target.Opts.ForceProbe {
		t.Errorf("target = %+v, want force-probe target", target)
	}
}

func TestCreateConfigTargetJSONStdin(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	oldStdin := os.Stdin
	os.Stdin = r
	t.Cleanup(func() {
		os.Stdin = oldStdin
		r.Close()
	})
	if _, err := w.WriteString(`{"Get":["baudRate"]}`); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	target, err := createConfigTarget(&flagVars{targetJSON: "-"})
	if err != nil {
		t.Fatalf("createConfigTarget: %v", err)
	}
	if target.Get != gpsprot.PropIDBaudRate {
		t.Errorf("Get = %v, want baudRate", target.Get)
	}
}

func TestCreateConfigTargetJSONTrailingData(t *testing.T) {
	for _, s := range []string{`{} {}`, `{} trailing`} {
		if _, err := createConfigTarget(&flagVars{targetJSON: s}); err == nil {
			t.Errorf("createConfigTarget(%q) succeeded, want error", s)
		}
	}
}

func TestCreateConfigTargetJSONMergesGet(t *testing.T) {
	v, _, err := parseFlags("gps", []string{"-d", "/dev/ttyACM0", "--target-json", `{"Get":["baudRate"]}`, "--show-config"})
	if err != nil {
		t.Fatalf("parseFlags: %v", err)
	}
	target, err := createConfigTarget(v)
	if err != nil {
		t.Fatalf("createConfigTarget: %v", err)
	}
	want := showProps | gpsprot.PropIDBaudRate
	if target.Get != want {
		t.Errorf("Get = %v, want %v", target.Get, want)
	}
}

func TestCreateConfigTargetJSONUnknownField(t *testing.T) {
	for _, s := range []string{
		`{"Unknown":true}`,
		`{"Props":{"unknown":true}}`,
		`{"Opts":{"Unknown":true}}`,
	} {
		if _, err := createConfigTarget(&flagVars{targetJSON: s}); err == nil {
			t.Errorf("createConfigTarget(%s) succeeded, want error", s)
		}
	}
}

func TestCreateConfigTargetJSONReadbackProps(t *testing.T) {
	var props gpsprot.ConfigProps
	props.SetSignalsEnabled(gpsprot.SignalSetOf(gpsprot.SigGPSL1CA))
	props.SetPort("UART1")
	b, err := json.Marshal(&props)
	if err != nil {
		t.Fatal(err)
	}
	target, err := createConfigTarget(&flagVars{targetJSON: fmt.Sprintf(`{"Props":%s}`, b)})
	if err != nil {
		t.Fatalf("createConfigTarget: %v", err)
	}
	if target.Props.ReadOnlyProps() != 0 {
		t.Errorf("read-only properties not cleared: %v", target.Props.ReadOnlyProps())
	}
	if got, ok := target.Props.GetSignalsEnabled(); !ok || got != gpsprot.SignalSetOf(gpsprot.SigGPSL1CA) {
		t.Errorf("signalsEnabled = %v, %t", got, ok)
	}
}

func TestCreateConfigTargetJSONDoesNotApplyFlagProps(t *testing.T) {
	v := flagVars{targetJSON: `{}`}
	v.pps.Set(time.Second)
	target, err := createConfigTarget(&v)
	if err != nil {
		t.Fatalf("createConfigTarget: %v", err)
	}
	if _, ok := target.Props.GetTimePulseWidth(); ok {
		t.Error("JSON target includes flag-derived PPS property")
	}
}

func TestTargetJSONShowPortConfigSupport(t *testing.T) {
	v, _, err := parseFlags("gps", []string{"-d", "/dev/ttyACM0", "--target-json", `{}`, "--show-port"})
	if err != nil {
		t.Fatalf("parseFlags: %v", err)
	}
	all, _ := v.configSupport.flags()
	if all != gpsprot.ConfigSupportPort {
		t.Errorf("configSupport = %v, want port", all.Items())
	}
}

func TestTargetJSONConfigFlagExclusive(t *testing.T) {
	_, _, err := parseFlags("gps", []string{"-d", "/dev/ttyACM0", "--target-json", `{}`, "--gnss", "GPS"})
	if err == nil {
		t.Fatal("parseFlags succeeded, want error")
	}
	if !strings.Contains(err.Error(), "--target-json cannot be combined with --gnss") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestTargetJSONShowTagsExclusive(t *testing.T) {
	_, _, err := parseFlags("gps", []string{"--target-json", `{}`, "--msg-file", "messages.toml", "--show-tags"})
	if err == nil {
		t.Fatal("parseFlags succeeded, want error")
	}
	if !strings.Contains(err.Error(), "--target-json cannot be combined with --msg-file") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestPrintConfigSupport(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "receiver-info-*.txt")
	if err != nil {
		t.Fatal(err)
	}
	printConfigSupport(f, gpsprot.ConfigSupportSignal|gpsprot.ConfigSupportSpeed)
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(f.Name())
	if err != nil {
		t.Fatal(err)
	}
	want := "Supports: signal, speed\n"
	if got := string(b); got != want {
		t.Errorf("printConfigSupport output = %q, want %q", got, want)
	}
}

func TestWarnMissingConfigSupport(t *testing.T) {
	var req configSupportReq
	req.require(gpsprot.ConfigSupportFixedPos, "--fixed-pos-ecef")
	req.require(gpsprot.ConfigSupportRaw, "--raw-out")
	req.require(gpsprot.ConfigSupportPort, "--show-port")
	req.requireMSM("--rtcm-out")
	var b bytes.Buffer
	lg := slog.New(slog.NewTextHandler(&b, nil))
	warnMissingConfigSupport(lg, req, gpsprot.ConfigSupportRaw|gpsprot.ConfigSupportRTCMMSM7)
	s := b.String()
	if !strings.Contains(s, `msg="receiver does not support the following option"`) {
		t.Errorf("log output missing warning message: %q", s)
	}
	if !strings.Contains(s, "option=--fixed-pos-ecef") {
		t.Errorf("log output missing option: %q", s)
	}
	if !strings.Contains(s, "option=--show-port") {
		t.Errorf("log output missing show-port option: %q", s)
	}
	if strings.Contains(s, "option=--raw-out") || strings.Contains(s, "option=--rtcm-out") {
		t.Errorf("log output contains supported option: %q", s)
	}
}
