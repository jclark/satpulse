package gpscmd

import (
	"fmt"
	"strings"

	"github.com/jclark/satpulse/internal/cmd/tool"
	"github.com/jclark/satpulse/internal/gpsprot"
	"github.com/spf13/pflag"
)

type flagVars struct {
	flash           bool
	reset           bool
	pps             bool
	nmea            bool
	force           bool
	localSpeed      int
	remoteSpeed     int
	serialDevice    string
	socketPath      string
	primaryGNSS     gpsprot.GNSS
	enabledGNSS     gpsprot.GNSSSet // avoid using a slice here, so flagVars is comparable
	disableTimeMode bool
	survey          bool
	surveyTime      uint32
	surveyAcc       float64
}

const summary = `[-h|--help] [-d|--serial-device path] [-s|--device-speed bps] [--socket path]
            [--flash] [--reset] [--speed bps] [--nmea] [--force] [-p|--pps]
			[-g|--gnss GPS|GAL|BDS|GLO|QZSS|NAVIC|SBAS,...] [--disable-time-mode]`

const defaultSurveyTime = 2000
const defaultSurveyAcc = 20.0

func parseFlags(cmdName string, args []string) (*flagVars, func(string) string, error) {
	help := false
	vars := flagVars{}
	gl := gnssList{}

	flags := pflag.NewFlagSet(cmdName, pflag.ContinueOnError)

	flags.BoolVarP(&help, "help", "h", false, "show help")
	flags.BoolVar(&vars.flash, "flash", false, "save the configuration changes to flash memory on the GPS receiver")
	flags.BoolVar(&vars.reset, "reset", false, "reset the GPS receiver")
	flags.BoolVar(&vars.nmea, "nmea", false, "enable NMEA output from the GPS receiver")
	flags.BoolVar(&vars.force, "force", false, "force writing to serial device even if no GPS detected")
	flags.StringVarP(&vars.serialDevice, "serial-device", "d", "", "serial device to configure")
	flags.StringVar(&vars.socketPath, "socket", "", "`path` of socket to connect to GPS receiver")
	flags.IntVarP(&vars.localSpeed, "device-speed", "s", 0, "serial device baud-rate in `bps`")
	flags.IntVar(&vars.remoteSpeed, "speed", 0, "set GPS receiver baud-rate in `bps`")
	flags.VarP(&gl, "gnss", "g", "enabled GNSS constellations `list`: GPS|GAL|BDS|GLO|QZSS|NAVIC|SBAS,...")
	flags.BoolVarP(&vars.pps, "pps", "p", false, "configure the GPS receiver to enable a PPS signal")
	flags.BoolVar(&vars.disableTimeMode, "disable-time-mode", false, "disable time mode")
	flags.BoolVar(&vars.survey, "survey", false, "instruct the GPS receiver to perform a survey")
	flags.Uint32Var(&vars.surveyTime, "survey-time", defaultSurveyTime, "survey time in seconds")
	flags.Float64Var(&vars.surveyAcc, "survey-acc", defaultSurveyAcc, "survey accuracy in meters")
	usage := tool.UsageFunc(cmdName, summary, flags)
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
	if len(gl.gnss) != 0 {
		primary := gl.gnss[0]
		if !primary.IsMajor() {
			return nil, nil, fmt.Errorf("first GNSS must be a major constellation: %s is not", primary)
		}
		vars.primaryGNSS = primary
		vars.enabledGNSS = gpsprot.GNSSFlag(gl.gnss...)
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
