package gpscmd

import (
	"bytes"
	"log/slog"
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

type trustedTimeBuilderFunc func(*gpsprot.TimeEstimate, time.Time) ([]byte, error)

func (f trustedTimeBuilderFunc) TrustedTimePacket(est *gpsprot.TimeEstimate, now time.Time) ([]byte, error) {
	return f(est, now)
}

func TestSendTrustedTime(t *testing.T) {
	est := &gpsprot.TimeEstimate{EstimatedTime: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)}
	want := []byte{0x01, 0x02, 0x03}
	var gotEst *gpsprot.TimeEstimate
	builder := trustedTimeBuilderFunc(func(est *gpsprot.TimeEstimate, now time.Time) ([]byte, error) {
		gotEst = est
		if now.IsZero() {
			t.Fatal("builder got zero build time")
		}
		return want, nil
	})
	var out bytes.Buffer
	if err := sendTrustedTime(slog.Default(), &out, builder, est); err != nil {
		t.Fatalf("sendTrustedTime() error = %v", err)
	}
	if gotEst != est {
		t.Fatalf("builder got estimate %p, want %p", gotEst, est)
	}
	if !bytes.Equal(out.Bytes(), want) {
		t.Fatalf("written packet = %x, want %x", out.Bytes(), want)
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
