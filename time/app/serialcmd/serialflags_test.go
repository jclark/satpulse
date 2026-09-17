package serialcmd

import (
	"testing"
	"time"

	"github.com/jclark/satpulse/gps/app/gpsio"
	"github.com/jclark/satpulse/gps/lib/opt"
)

func TestParseFlags(t *testing.T) {
	for _, tc := range []struct {
		name     string
		args     []string
		want     flagVars
		wantErr  bool
		wantHelp bool
	}{
		{name: "default info all", want: flagVars{info: true, all: true}},
		{name: "jsonl info all", args: []string{"-j"}, want: flagVars{jsonl: true, info: true, all: true}},
		{name: "explicit info all", args: []string{"-i"}, want: flagVars{info: true, all: true}},
		{name: "info selected", args: []string{"-i", "-d", "/dev/ttyS0"}, want: flagVars{info: true, device: "/dev/ttyS0"}},
		{name: "detect all", args: []string{"-a"}, want: flagVars{all: true}},
		{name: "detect selected", args: []string{"-d", "/dev/ttyS0"}, want: flagVars{device: "/dev/ttyS0"}},
		{name: "detect with packet log", args: []string{"-d", "/dev/ttyS0", "--packet-log", "capture.jsonl"}, want: flagVars{device: "/dev/ttyS0", packetLog: "capture.jsonl"}},
		{name: "capture at speed", args: []string{"-j", "-s", "38400", "-d", "/dev/ttyS0", "--packet-log", "capture.jsonl"}, want: flagVars{jsonl: true, device: "/dev/ttyS0", deviceSpeed: opt.Make(38400), packetLog: "capture.jsonl"}},
		{name: "timed capture", args: []string{"-d", "/dev/ttyS0", "-s", "38400", "--packet-log", "capture.jsonl", "-t", "1.25"}, want: flagVars{device: "/dev/ttyS0", deviceSpeed: opt.Make(38400), packetLog: "capture.jsonl", timeout: 1250 * time.Millisecond}},
		{name: "capture at current speed", args: []string{"-s", "0", "-d", "/dev/ttyS0", "--packet-log", "capture.jsonl"}, want: flagVars{device: "/dev/ttyS0", deviceSpeed: opt.Make(0), packetLog: "capture.jsonl"}},
		{name: "pps on cts", args: []string{"-p", "cts", "-d", "/dev/ttyS0"}, want: flagVars{device: "/dev/ttyS0", ppsPin: opt.Make(gpsio.SerialPinCTS), timeout: defaultPPSTimeout}},
		{name: "pps all ports", args: []string{"--pps-pin", "dcd", "-a"}, want: flagVars{all: true, ppsPin: opt.Make(gpsio.SerialPinDCD), timeout: defaultPPSTimeout}},
		{name: "pps pin uppercase", args: []string{"-p", "CTS", "-d", "/dev/ttyS0"}, want: flagVars{device: "/dev/ttyS0", ppsPin: opt.Make(gpsio.SerialPinCTS), timeout: defaultPPSTimeout}},
		{name: "pps at speed", args: []string{"-p", "ri", "-s", "38400", "-d", "/dev/ttyS0"}, want: flagVars{device: "/dev/ttyS0", ppsPin: opt.Make(gpsio.SerialPinRI), deviceSpeed: opt.Make(38400), timeout: defaultPPSTimeout}},
		{name: "pps with packet log and timeout", args: []string{"-p", "dsr", "-d", "/dev/ttyS0", "--packet-log", "capture.jsonl", "-t", "30"}, want: flagVars{device: "/dev/ttyS0", ppsPin: opt.Make(gpsio.SerialPinDSR), packetLog: "capture.jsonl", timeout: 30 * time.Second}},
		{name: "pps until interrupted", args: []string{"-p", "cts", "-t", "0", "-d", "/dev/ttyS0"}, want: flagVars{device: "/dev/ttyS0", ppsPin: opt.Make(gpsio.SerialPinCTS)}},
		{name: "pps JSONL", args: []string{"-j", "-p", "cts", "-a"}, want: flagVars{jsonl: true, all: true, ppsPin: opt.Make(gpsio.SerialPinCTS), timeout: defaultPPSTimeout}},
		{name: "pps inverted polarity", args: []string{"-p", "cts", "--invert-polarity", "-d", "/dev/ttyS0"}, want: flagVars{device: "/dev/ttyS0", ppsPin: opt.Make(gpsio.SerialPinCTS), invertPolarity: true, timeout: defaultPPSTimeout}},
		{name: "pps by polling", args: []string{"-p", "cts", "-m", "poll", "-d", "/dev/ttyS0"}, want: flagVars{device: "/dev/ttyS0", ppsPin: opt.Make(gpsio.SerialPinCTS), ppsMethod: gpsio.PPSMethodPoll, timeout: defaultPPSTimeout}},
		{name: "pps by waiting", args: []string{"-p", "cts", "--pps-method", "wait", "-d", "/dev/ttyS0"}, want: flagVars{device: "/dev/ttyS0", ppsPin: opt.Make(gpsio.SerialPinCTS), ppsMethod: gpsio.PPSMethodWait, timeout: defaultPPSTimeout}},
		{name: "pps with kernel timestamps", args: []string{"-p", "dcd", "--pps-method", "kernel", "-d", "/dev/ttyS0"}, want: flagVars{device: "/dev/ttyS0", ppsPin: opt.Make(gpsio.SerialPinDCD), ppsMethod: gpsio.PPSMethodKernel, timeout: defaultPPSTimeout}},
		{name: "pps with wakeup latency", args: []string{"-p", "cts", "--max-wakeup-latency", "10.5e-6", "-d", "/dev/ttyS0"}, want: flagVars{device: "/dev/ttyS0", ppsPin: opt.Make(gpsio.SerialPinCTS), maxWakeupLatency: opt.Make(10.5e-6), timeout: defaultPPSTimeout}},
		{name: "pps with zero wakeup latency", args: []string{"-p", "cts", "--max-wakeup-latency", "0", "-d", "/dev/ttyS0"}, want: flagVars{device: "/dev/ttyS0", ppsPin: opt.Make(gpsio.SerialPinCTS), maxWakeupLatency: opt.Make(0.0), timeout: defaultPPSTimeout}},
		{name: "pps with outlier ratio", args: []string{"-p", "cts", "--poll-outlier-ratio", "4", "-d", "/dev/ttyS0"}, want: flagVars{device: "/dev/ttyS0", ppsPin: opt.Make(gpsio.SerialPinCTS), pollOutlierRatio: opt.Make(4.0), timeout: defaultPPSTimeout}},
		{name: "pps with outlier check disabled", args: []string{"-p", "cts", "--poll-outlier-ratio", "0", "-d", "/dev/ttyS0"}, want: flagVars{device: "/dev/ttyS0", ppsPin: opt.Make(gpsio.SerialPinCTS), pollOutlierRatio: opt.Make(0.0), timeout: defaultPPSTimeout}},
		{name: "help", args: []string{"-h"}, wantHelp: true},
		{name: "positional port", args: []string{"/dev/ttyS0"}, wantErr: true},
		{name: "all and device", args: []string{"-a", "-d", "/dev/ttyS0"}, wantErr: true},
		{name: "info and speed", args: []string{"-i", "-d", "/dev/ttyS0", "-s", "38400", "--packet-log", "capture.jsonl"}, wantErr: true},
		{name: "info and packet log", args: []string{"-i", "-d", "/dev/ttyS0", "--packet-log", "capture.jsonl"}, wantErr: true},
		{name: "info and timeout", args: []string{"-i", "-d", "/dev/ttyS0", "-t", "1"}, wantErr: true},
		{name: "info and pps", args: []string{"-i", "-p", "cts", "-d", "/dev/ttyS0"}, wantErr: true},
		{name: "invert polarity without pps", args: []string{"--invert-polarity", "-d", "/dev/ttyS0"}, wantErr: true},
		{name: "method without pps", args: []string{"-m", "poll", "-d", "/dev/ttyS0"}, wantErr: true},
		{name: "invalid method", args: []string{"-p", "cts", "-m", "sideband", "-d", "/dev/ttyS0"}, wantErr: true},
		{name: "wakeup latency without pps", args: []string{"--max-wakeup-latency", "10e-6", "-d", "/dev/ttyS0"}, wantErr: true},
		{name: "negative wakeup latency", args: []string{"-p", "cts", "--max-wakeup-latency", "-1", "-d", "/dev/ttyS0"}, wantErr: true},
		{name: "NaN wakeup latency", args: []string{"-p", "cts", "--max-wakeup-latency", "NaN", "-d", "/dev/ttyS0"}, wantErr: true},
		{name: "infinite wakeup latency", args: []string{"-p", "cts", "--max-wakeup-latency", "+Inf", "-d", "/dev/ttyS0"}, wantErr: true},
		{name: "one-second wakeup latency", args: []string{"-p", "cts", "--max-wakeup-latency", "1", "-d", "/dev/ttyS0"}, wantErr: true},
		{name: "outlier ratio without pps", args: []string{"--poll-outlier-ratio", "4", "-d", "/dev/ttyS0"}, wantErr: true},
		{name: "outlier ratio below one", args: []string{"-p", "cts", "--poll-outlier-ratio", "0.5", "-d", "/dev/ttyS0"}, wantErr: true},
		{name: "NaN outlier ratio", args: []string{"-p", "cts", "--poll-outlier-ratio", "NaN", "-d", "/dev/ttyS0"}, wantErr: true},
		{name: "infinite outlier ratio", args: []string{"-p", "cts", "--poll-outlier-ratio", "+Inf", "-d", "/dev/ttyS0"}, wantErr: true},
		{name: "pps without target", args: []string{"-p", "cts"}, wantErr: true},
		{name: "pps invalid pin", args: []string{"-p", "rts", "-d", "/dev/ttyS0"}, wantErr: true},
		{name: "pps value required", args: []string{"--pps-pin", "-d", "/dev/ttyS0"}, wantErr: true},
		{name: "pps speed without device", args: []string{"-a", "-p", "cts", "-s", "38400"}, wantErr: true},
		{name: "pps packet log with all", args: []string{"-a", "-p", "cts", "--packet-log", "capture.jsonl"}, wantErr: true},
		{name: "speed without device", args: []string{"-s", "38400", "--packet-log", "capture.jsonl"}, wantErr: true},
		{name: "speed with all", args: []string{"-a", "-s", "38400", "--packet-log", "capture.jsonl"}, wantErr: true},
		{name: "speed without purpose", args: []string{"-d", "/dev/ttyS0", "-s", "38400"}, wantErr: true},
		{name: "negative speed", args: []string{"-d", "/dev/ttyS0", "-s", "-1", "--packet-log", "capture.jsonl"}, wantErr: true},
		{name: "non-numeric speed", args: []string{"-d", "/dev/ttyS0", "-s", "auto", "--packet-log", "capture.jsonl"}, wantErr: true},
		{name: "packet log without device", args: []string{"-a", "--packet-log", "capture.jsonl"}, wantErr: true},
		{name: "timeout without purpose", args: []string{"-d", "/dev/ttyS0", "-t", "1"}, wantErr: true},
		{name: "timeout with detection", args: []string{"-d", "/dev/ttyS0", "--packet-log", "capture.jsonl", "-t", "1"}, wantErr: true},
		{name: "negative timeout", args: []string{"-d", "/dev/ttyS0", "-s", "38400", "--packet-log", "capture.jsonl", "-t", "-1"}, wantErr: true},
		{name: "negative pps timeout", args: []string{"-p", "cts", "-d", "/dev/ttyS0", "-t", "-1"}, wantErr: true},
		{name: "NaN timeout", args: []string{"-d", "/dev/ttyS0", "-s", "38400", "--packet-log", "capture.jsonl", "-t", "NaN"}, wantErr: true},
		{name: "infinite timeout", args: []string{"-d", "/dev/ttyS0", "-s", "38400", "--packet-log", "capture.jsonl", "-t", "+Inf"}, wantErr: true},
		{name: "overflowing timeout", args: []string{"-d", "/dev/ttyS0", "-s", "38400", "--packet-log", "capture.jsonl", "-t", "1e20"}, wantErr: true},
		{name: "underflowing timeout", args: []string{"-d", "/dev/ttyS0", "-s", "38400", "--packet-log", "capture.jsonl", "-t", "1e-10"}, wantErr: true},
		{name: "empty device", args: []string{"--serial-device", ""}, wantErr: true},
		{name: "empty packet log", args: []string{"--serial-device", "/dev/ttyS0", "--packet-log", ""}, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, help, _, err := parseFlags("serial", tc.args)
			if (err != nil) != tc.wantErr {
				t.Fatalf("parseFlags() error = %v, wantErr %v", err, tc.wantErr)
			}
			if help != tc.wantHelp {
				t.Errorf("help = %v, want %v", help, tc.wantHelp)
			}
			if !tc.wantErr && got != tc.want {
				t.Errorf("flags = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestParseFlagsTimeoutDependencies(t *testing.T) {
	const want = "--timeout requires --pps-pin, or --device-speed and --packet-log"
	for _, args := range [][]string{
		{"-d", "/dev/ttyS0", "-t", "1"},
		{"-d", "/dev/ttyS0", "-s", "38400", "-t", "1"},
		{"-d", "/dev/ttyS0", "--packet-log", "capture.jsonl", "-t", "1"},
	} {
		_, _, _, err := parseFlags("serial", args)
		if err == nil || err.Error() != want {
			t.Errorf("parseFlags(%q) error = %v, want %q", args, err, want)
		}
	}
}
