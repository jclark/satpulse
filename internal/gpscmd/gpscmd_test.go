package gpscmd

import (
	"bytes"
	"log/slog"
	"os"
	"strings"
	"testing"

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

func TestPrintConfigSupport(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "receiver-info-*.txt")
	if err != nil {
		t.Fatal(err)
	}
	printConfigSupport(f, gpsprot.ConfigSupportBand|gpsprot.ConfigSupportSpeed)
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(f.Name())
	if err != nil {
		t.Fatal(err)
	}
	want := "Supports: band, speed\n"
	if got := string(b); got != want {
		t.Errorf("printConfigSupport output = %q, want %q", got, want)
	}
}

func TestWarnMissingConfigSupport(t *testing.T) {
	var req configSupportReq
	req.require(gpsprot.ConfigSupportFixedPos, "--fixed-pos-ecef")
	req.require(gpsprot.ConfigSupportRaw, "--raw-out")
	req.require(gpsprot.ConfigSupportReload, "--reload")
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
	if !strings.Contains(s, "option=--reload") {
		t.Errorf("log output missing reload option: %q", s)
	}
	if !strings.Contains(s, "option=--show-port") {
		t.Errorf("log output missing show-port option: %q", s)
	}
	if strings.Contains(s, "option=--raw-out") || strings.Contains(s, "option=--rtcm-out") {
		t.Errorf("log output contains supported option: %q", s)
	}
}
