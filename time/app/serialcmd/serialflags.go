package serialcmd

import (
	"fmt"
	"math"
	"time"

	"github.com/jclark/satpulse/gps/app/cmd"
	"github.com/jclark/satpulse/gps/app/gpsio"
	"github.com/jclark/satpulse/gps/app/serialpps"
	"github.com/jclark/satpulse/gps/lib/opt"
	"github.com/jclark/satpulse/gps/ptime"
	"github.com/spf13/pflag"
)

type flagVars struct {
	all              bool
	device           string
	info             bool
	ppsPin           opt.Val[gpsio.SerialPin]
	invertPolarity   bool
	ppsMethod        gpsio.PPSMethod
	pollPreWarm      float64
	maxWakeupLatency opt.Val[float64]
	packetLog        string
	deviceSpeed      opt.Val[int]
	timeout          time.Duration
	jsonl            bool
}

const summary = `[-h|--help] [-a|--all | -d|--serial-device path] [-i|--info]
              [-p|--pps-pin pin] [-I|--invert-polarity] [-m|--pps-method method]
              [--poll-pre-warm seconds]
              [--max-wakeup-latency seconds] [--packet-log path]
              [-s|--device-speed bps]
              [-t|--timeout seconds] [-j|--jsonl]`

const defaultPPSTimeout = 10 * time.Second

func parseFlags(cmdName string, args []string) (v flagVars, help bool, usageFunc func(string) string, err error) {
	flags := pflag.NewFlagSet(cmdName, pflag.ContinueOnError)
	flags.SortFlags = false
	var ppsPinName, ppsMethodName string
	var maxWakeupLatency, timeoutSec float64
	var deviceSpeed int
	flags.BoolVarP(&help, "help", "h", false, "show usage help for the serial command")
	flags.BoolVarP(&v.all, "all", "a", false, "operate on all discovered serial ports")
	flags.StringVarP(&v.device, "serial-device", "d", "", "operate on the serial port at `path`")
	flags.BoolVarP(&v.info, "info", "i", false, "show information about serial ports without opening them")
	flags.StringVarP(&ppsPinName, "pps-pin", "p", "", "detect PPS edges on the modem-control input `pin` (cts, dcd, dsr, or ri)")
	flags.BoolVarP(&v.invertPolarity, "invert-polarity", "I", false, "invert the usual PPS pulse polarity, for edges that trail the start of the second by the pulse width")
	flags.StringVarP(&ppsMethodName, "pps-method", "m", "", "force the PPS edge detection `method` (poll, wait, or kernel)")
	flags.Float64Var(&v.pollPreWarm, "poll-pre-warm", 0, "busy-wait `seconds` before each poll window, for hosts whose reads slow down while idle")
	flags.Float64Var(&maxWakeupLatency, "max-wakeup-latency", 0, "limit CPU wakeup latency to `seconds` while detecting PPS")
	flags.StringVar(&v.packetLog, "packet-log", "", "write received packets to a JSON Lines log at `path`")
	flags.IntVarP(&deviceSpeed, "device-speed", "s", 0, "set the serial port speed to `bps` (0 uses the current speed)")
	flags.Float64VarP(&timeoutSec, "timeout", "t", 0, "stop detecting PPS edges or capturing packets after `seconds` (0 = until interrupted)")
	flags.BoolVarP(&v.jsonl, "jsonl", "j", false, "write output in JSON Lines format")
	usageFunc = cmd.UsageFunc(cmdName, summary, flags)
	if err = flags.Parse(args); err != nil || help {
		return
	}
	if flags.NArg() != 0 {
		err = fmt.Errorf("serial command does not accept positional arguments")
		return
	}
	deviceSet := flags.Lookup("serial-device").Changed
	if flags.Lookup("device-speed").Changed {
		v.deviceSpeed.Set(deviceSpeed)
	}
	timeoutSet := flags.Lookup("timeout").Changed
	packetLogSet := flags.Lookup("packet-log").Changed
	methodSet := flags.Lookup("pps-method").Changed
	preWarmSet := flags.Lookup("poll-pre-warm").Changed
	if flags.Lookup("max-wakeup-latency").Changed {
		v.maxWakeupLatency.Set(maxWakeupLatency)
	}
	if flags.Lookup("pps-pin").Changed {
		var pin gpsio.SerialPin
		if pin, err = gpsio.ParseSerialPin(ppsPinName); err != nil {
			err = fmt.Errorf("--pps-pin must be one of cts, dcd, dsr, or ri")
			return
		}
		v.ppsPin.Set(pin)
	}
	if methodSet {
		if v.ppsMethod, err = gpsio.ParsePPSMethod(ppsMethodName); err != nil {
			err = fmt.Errorf("--pps-method must be one of poll, wait, or kernel")
			return
		}
	}
	if preWarmSet {
		if err = checkNonNegative(v.pollPreWarm, 1, "--poll-pre-warm"); err != nil {
			return
		}
	}
	if v.maxWakeupLatency.IsSet() {
		if err = checkNonNegative(v.maxWakeupLatency.Get(), 1, "--max-wakeup-latency"); err != nil {
			return
		}
	}
	if deviceSet && v.device == "" {
		err = fmt.Errorf("--serial-device must not be empty")
		return
	}
	if v.all && deviceSet {
		err = fmt.Errorf("--all cannot be combined with --serial-device")
		return
	}
	if v.deviceSpeed.IsSet() && v.deviceSpeed.Get() < 0 {
		err = fmt.Errorf("--device-speed must not be negative")
		return
	}
	if timeoutSet {
		switch {
		case math.IsNaN(timeoutSec) || math.IsInf(timeoutSec, 0):
			err = fmt.Errorf("--timeout must be finite")
			return
		case timeoutSec < 0:
			err = fmt.Errorf("--timeout must not be negative")
			return
		case timeoutSec >= float64(math.MaxInt64)/float64(time.Second):
			err = fmt.Errorf("--timeout is too large")
			return
		case timeoutSec > 0 && ptime.Seconds(timeoutSec) == 0:
			err = fmt.Errorf("--timeout is too small")
			return
		}
	}
	if packetLogSet && v.packetLog == "" {
		err = fmt.Errorf("--packet-log must not be empty")
		return
	}
	if v.info && (v.ppsPin.IsSet() || v.invertPolarity || methodSet || preWarmSet || v.maxWakeupLatency.IsSet() || v.deviceSpeed.IsSet() || timeoutSet || packetLogSet) {
		err = fmt.Errorf("--info cannot be combined with --pps-pin, --invert-polarity, --pps-method, --poll-pre-warm, --max-wakeup-latency, --device-speed, --timeout, or --packet-log")
		return
	}
	if v.ppsPin.IsSet() && !deviceSet && !v.all {
		err = fmt.Errorf("--pps-pin requires --serial-device or --all")
		return
	}
	if v.invertPolarity && !v.ppsPin.IsSet() {
		err = fmt.Errorf("--invert-polarity requires --pps-pin")
		return
	}
	if methodSet && !v.ppsPin.IsSet() {
		err = fmt.Errorf("--pps-method requires --pps-pin")
		return
	}
	if preWarmSet && !v.ppsPin.IsSet() {
		err = fmt.Errorf("--poll-pre-warm requires --pps-pin")
		return
	}
	if v.maxWakeupLatency.IsSet() && !v.ppsPin.IsSet() {
		err = fmt.Errorf("--max-wakeup-latency requires --pps-pin")
		return
	}
	if timeoutSet && !v.ppsPin.IsSet() && !(v.deviceSpeed.IsSet() && packetLogSet) {
		err = fmt.Errorf("--timeout requires --pps-pin, or --device-speed and --packet-log")
		return
	}
	if v.deviceSpeed.IsSet() && !deviceSet {
		err = fmt.Errorf("--device-speed requires --serial-device")
		return
	}
	if v.deviceSpeed.IsSet() && !v.ppsPin.IsSet() && !packetLogSet {
		err = fmt.Errorf("--device-speed requires --pps-pin or --packet-log")
		return
	}
	if packetLogSet && !deviceSet {
		err = fmt.Errorf("--packet-log requires --serial-device")
		return
	}
	if timeoutSet {
		v.timeout = ptime.Seconds(timeoutSec)
	} else if v.ppsPin.IsSet() {
		v.timeout = defaultPPSTimeout
	}
	if !deviceSet && !v.all {
		v.info = true
		v.all = true
	}
	return
}

// checkNonNegative rejects val outside [0, limit); NaN fails both comparisons.
func checkNonNegative(val, limit float64, flag string) error {
	if val >= 0 && val < limit {
		return nil
	}
	return fmt.Errorf("%s must be at least 0 and less than %g", flag, limit)
}

// ppsConfig is the detection configuration the PPS flags describe; the other
// Config fields matter only when edges are turned into samples.
func (v flagVars) ppsConfig() serialpps.Config {
	cfg := serialpps.Config{Method: v.ppsMethod, PollPreWarm: v.pollPreWarm}
	cfg.MaxWakeupLatency = v.maxWakeupLatency.Ptr()
	return cfg
}

// wiring is the pulse wiring the PPS flags describe.
func (v flagVars) wiring() serialpps.Wiring {
	w := serialpps.Wiring{Pin: v.ppsPin.Get()}
	if v.invertPolarity {
		w.Polarity = serialpps.PolarityAssert
	}
	return w
}
