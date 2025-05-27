package gpscmd

import (
	"fmt"
	"strings"

	"github.com/jclark/satpulse/internal/cmd"
	"github.com/jclark/satpulse/internal/gpsprot"
	"github.com/spf13/pflag"
)

type packetLogMode int

const (
	packetsOnlyLogMode packetLogMode = iota
	testLogMode
)

type flagVars struct {
	save            gpsprot.SaveType
	reset           gpsprot.ResetType
	pps             bool
	nmea            bool
	binary          bool
	forceProbe      bool
	localSpeed      int
	remoteSpeed     int
	serialDevice    string
	socketPath      string
	primaryGNSS     gpsprot.GNSS
	enabledSignals  gpsprot.SignalSet
	disableTimeMode bool
	survey          bool
	surveyTime      uint32
	surveyAcc       float64
	packetLogPath   string
	packetLogMode   packetLogMode
}

const summary = `[-h|--help] [-d|--serial-device path] [-s|--device-speed bps] [--force-probe]
       	    [--socket path] [--packet-log path] [--save] [--speed bps] [--nmea] [--binary]
            [--save] [--save-all] [--reset] [--reload] [--factory-reset]
            [-g|--gnss GPS|GAL|BDS|GLO|QZSS|NAVIC|SBAS,...] [-b|--band L1|L2|L5|E5|L6,...]`

const defaultSurveyTime = 2000
const defaultSurveyAcc = 20.0

func parseFlags(cmdName string, args []string) (*flagVars, func(string) string, error) {
	help := false
	vars := flagVars{}
	gl := gnssList{}
	bands := bands(gpsprot.BandAll)
	save := false
	saveAll := false
	reset := false
	factoryReset := false
	reload := false
	testLogPath := ""

	flags := pflag.NewFlagSet(cmdName, pflag.ContinueOnError)

	flags.BoolVarP(&help, "help", "h", false, "show help")
	flags.BoolVar(&save, "save", false, "save configuration changes to non-volatile memory on the GPS receiver")
	flags.BoolVar(&saveAll, "save-all", false, "save the current configuration to non-volatile memory on the GPS receiver")
	flags.BoolVar(&reset, "reset", false, "reset the GPS receiver and perform a cold start")
	flags.BoolVar(&reload, "reload", false, "reload the GPS receiver configuration from non-volatile memory")
	flags.BoolVar(&factoryReset, "factory-reset", false, "reset the GPS receiver to factory defaults")
	flags.BoolVar(&vars.nmea, "nmea", false, "enable NMEA output from the GPS receiver")
	flags.BoolVar(&vars.binary, "binary", false, "enable binary output from the GPS receiver")
	flags.BoolVar(&vars.forceProbe, "force-probe", false, "force writing probe to serial device even if when no output from GPS receiver")
	flags.StringVarP(&vars.serialDevice, "serial-device", "d", "", "serial device connected to GPS receiver")
	flags.StringVar(&vars.socketPath, "socket", "", "`path` of socket to connect to GPS receiver")
	flags.StringVar(&vars.packetLogPath, "packet-log", "", "log packets to `path`")
	flags.StringVar(&testLogPath, "test-log", "", "log test data to `path`")
	flags.MarkHidden("test-log")
	flags.IntVarP(&vars.localSpeed, "device-speed", "s", 0, "serial device baud-rate in `bps`")
	flags.IntVar(&vars.remoteSpeed, "speed", 0, "set GPS receiver baud-rate in `bps`")
	flags.VarP(&gl, "gnss", "g", "enabled GNSS constellations `list`: GPS|GAL|BDS|GLO|QZSS|NAVIC|SBAS,...")
	flags.VarP(&bands, "band", "b", "enabled GNSS bands `list`: L1,L2,L5,E5,E6,...")
	flags.BoolVarP(&vars.pps, "pps", "p", false, "configure the GPS receiver to enable a PPS signal")
	flags.MarkHidden("pps")
	flags.BoolVar(&vars.disableTimeMode, "disable-time-mode", false, "disable time mode")
	flags.MarkHidden("disable-time-mode")
	flags.BoolVar(&vars.survey, "survey", false, "instruct the GPS receiver to perform a survey")
	flags.MarkHidden("survey")
	flags.Uint32Var(&vars.surveyTime, "survey-time", defaultSurveyTime, "survey time in seconds")
	flags.MarkHidden("survey-time")
	flags.Float64Var(&vars.surveyAcc, "survey-acc", defaultSurveyAcc, "survey accuracy in meters")
	flags.MarkHidden("survey-acc")
	usage := cmd.UsageFunc(cmdName, summary, flags)
	err := flags.Parse(args)
	if err != nil {
		return nil, usage, err
	}
	if help {
		return nil, usage, nil
	}
	if flags.NArg() != 0 {
		return nil, usage, fmt.Errorf("%s command must not have non-option arguments", cmdName)
	}
	if (vars.socketPath == "") == (vars.serialDevice == "") {
		return nil, usage, fmt.Errorf("%s command must specify either --socket or --serial-device", cmdName)
	}
	if testLogPath != "" {
		if vars.packetLogPath != "" {
			return nil, usage, fmt.Errorf("%s command must not specify both --packet-log and --test-log", cmdName)
		}
		vars.packetLogPath = testLogPath
		vars.packetLogMode = testLogMode
	}
	if vars.disableTimeMode && vars.survey {
		return nil, nil, fmt.Errorf("%s command must not specify both --disable-time-mode and --survey", cmdName)
	}
	if vars.surveyAcc < 0.001 {
		return nil, nil, fmt.Errorf("--survey-acc must at least 0.001 (1 mm)")
	}
	for _, s := range []string{"device-speed", "speed"} {
		f := flags.Lookup(s)
		if f.Changed && f.Value.String() == "0" {
			return nil, usage, fmt.Errorf("0 is not a valid value for --%s", s)
		}
	}
	configChanged := false
	if len(gl.gnss) != 0 {
		vars.enabledSignals = gpsprot.Band(bands).SignalSet(gl.gnss...)
		if (vars.enabledSignals&gpsprot.SigSetMajor)&^gpsprot.SigSetAugment == 0 {
			return nil, nil, fmt.Errorf("at least one non-augmentation signal from a major GNSS must be enabled")
		}
		configChanged = true
	} else if flags.Lookup("band").Changed {
		return nil, nil, fmt.Errorf("%s command must specify --gnss when --band is specified", cmdName)
	}
	if vars.remoteSpeed != 0 || vars.pps || vars.nmea || vars.binary || vars.primaryGNSS != 0 {
		configChanged = true
		if vars.nmea && vars.binary {
			return nil, nil, fmt.Errorf("%s command must not specify both --nmea and --binary options", cmdName)
		}
	}
	if save {
		if !configChanged {
			return nil, nil, fmt.Errorf("no configuration changes to save with --save; use --save-all to save current configuration")
		}
		if saveAll {
			return nil, nil, fmt.Errorf("%s command must not specify both --save and --save-all", cmdName)
		}
		vars.save = gpsprot.SaveMinimal
	} else if saveAll {
		vars.save = gpsprot.SaveAll
	}
	if factoryReset {
		if save || saveAll || reset || reload || configChanged {
			return nil, nil, fmt.Errorf("%s command must not use --factory-reset with --save, --save-all, --reset, --reload or configuration changes", cmdName)
		}
		vars.reset = gpsprot.ResetFactory
	} else if reset || reload {
		if configChanged && !save && !saveAll {
			return nil, nil, fmt.Errorf("--reset or --reload without saving would lose configuration changes")
		}
		if reset && reload {
			return nil, nil, fmt.Errorf("cannot use both --reset and --reload")
		}
		if reload {
			vars.reset = gpsprot.ResetReload
		} else {
			vars.reset = gpsprot.ResetCold
		}
	}
	return &vars, nil, nil
}

type gnssList struct {
	gnss []gpsprot.GNSS
}

var _ pflag.Value = (*gnssList)(nil)

func (gl *gnssList) String() string {
	var s []string
	for _, gnss := range gl.gnss {
		s = append(s, gnss.String())
	}
	return strings.Join(s, ",")
}

func (gl *gnssList) Type() string {
	return "gnss-list"
}

func (gl *gnssList) Set(s string) error {
	words := strings.Split(s, ",")
	for _, w := range words {
		gnss, err := gpsprot.ParseGNSS(strings.Trim(w, " \t"))
		if err != nil {
			return err
		}
		gl.gnss = append(gl.gnss, gnss)
	}
	return nil
}

type bands gpsprot.Band

var _ pflag.Value = (*bands)(nil)

var bandTable = []struct {
	name string
	band gpsprot.Band
}{
	{"L1", gpsprot.BandL1},
	{"L2", gpsprot.BandL2},
	{"E5", gpsprot.BandL5 | gpsprot.BandE5b}, // needs to be before L5 for String() to work
	{"L5", gpsprot.BandL5},
	{"E6", gpsprot.BandE6},
	{"L6", gpsprot.BandE6},
}

func (bp *bands) String() string {
	b := gpsprot.Band(*bp)
	if b == 0 {
		return "(none)"
	}
	if b == gpsprot.BandAll {
		return "all"
	}
	s := ""
	for _, bn := range bandTable {
		if bn.band&b == bn.band {
			s += bn.name + ","
			b &^= bn.band
		}
	}
	if b == 0 {
		return s[:len(s)-1]
	}
	return s + fmt.Sprintf("0x%4X", b)
}

func (bp *bands) Type() string {
	return "bands"
}

func (bp *bands) Set(s string) error {
	b := gpsprot.Band(0)
	words := strings.Split(s, ",")
	for _, w := range words {
		w := strings.Trim(w, " \t")
		found := false
		for _, bn := range bandTable {
			if strings.EqualFold(bn.name, w) {
				b |= bn.band
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("unknown band: %s", w)
		}
	}
	*bp = bands(b)
	return nil
}
