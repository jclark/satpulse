package gpscmd

import (
	"fmt"
	"strings"

	"github.com/jclark/satpulse/internal/cmd"
	"github.com/jclark/satpulse/internal/gpsevent"
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
	pvtMsg          gpsprot.PVTMsgFlags
	rawMsg          gpsprot.Option[gpsprot.RawMsgFlags]
	rtcmMsg         gpsprot.Option[gpsprot.RTCMMsgFlags]
	nmeaMsg         gpsprot.Option[gpsprot.NMEAMsgFlags]
	satsMsg         gpsprot.Option[gpsprot.SatsMsgFlags]
}

const summary = `[-h|--help] [-d|--serial-device path] [-s|--device-speed bps] [--force-probe]
       	    [--socket path] [--packet-log path] [--save] [--speed bps] [--nmea] [--binary]
            [--save] [--save-all] [--reset] [--reload] [--factory-reset]
            [-g|--gnss GPS|GAL|BDS|GLO|QZSS|NAVIC|SBAS,...] [-b|--band L1|L2|L5|E5|L6,...]
            [--raw-out obs|nav|none,...] [--pvt-out pos|vel|time|tp|leap|tai|ecef|off,...]
            [--rtcm-out MSM4|MSM7|ARP|none,...] [--nmea-out RMC|GGA|GSA|GSV|none,...]`

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
	var rawOut rawOutOpt
	var pvtOut pvtOutOpt
	var rtcmOut rtcmOutOpt
	var nmeaOut nmeaOutOpt
	nmea := false
	binary := false

	flags := pflag.NewFlagSet(cmdName, pflag.ContinueOnError)

	flags.BoolVarP(&help, "help", "h", false, "show help")
	flags.BoolVar(&save, "save", false, "save configuration changes to non-volatile memory on the GPS receiver")
	flags.BoolVar(&saveAll, "save-all", false, "save the current configuration to non-volatile memory on the GPS receiver")
	flags.BoolVar(&reset, "reset", false, "reset the GPS receiver and perform a cold start")
	flags.BoolVar(&reload, "reload", false, "reload the GPS receiver configuration from non-volatile memory")
	flags.BoolVar(&factoryReset, "factory-reset", false, "reset the GPS receiver to factory defaults")
	flags.BoolVar(&nmea, "nmea", false, "enable NMEA output and disable binary output from the GPS receiver")
	flags.BoolVar(&binary, "binary", false, "enable binary output and disable NMEA output from the GPS receiver")
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
	flags.Var(&rawOut, "raw-out", "raw data messages to output `flags`: obs|nav|none,...")
	flags.Var(&pvtOut, "pvt-out", "PVT messages to output `flags`: pos|vel|time|tp|leap|tai|ecef|daemon|off,...")
	flags.Var(&rtcmOut, "rtcm-out", "RTCM messages to output `flags`: MSM4|MSM7|ARP|none,...")
	flags.Var(&nmeaOut, "nmea-out", "NMEA messages to output `flags`: RMC|GGA|GSA|GSV|ZDA|none,...")
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
	pvtMsg := gpsprot.PVTMsgFlags(pvtOut)
	rawMsg := (gpsprot.Option[gpsprot.RawMsgFlags])(rawOut)
	rtcmMsg := (gpsprot.Option[gpsprot.RTCMMsgFlags])(rtcmOut)
	nmeaMsg := (gpsprot.Option[gpsprot.NMEAMsgFlags])(nmeaOut)

	if rawMsg.IsSet() || pvtMsg.IsSet() || rtcmMsg.IsSet() || nmeaMsg.IsSet() {
		configChanged = true
	}

	if nmea {
		configChanged = true
		// the concept of --nmea is to give NMEA output only
		// we require that there is some NMEA output
		// --nmea-out can be used to specify what; defaults to RMC
		if binary {
			return nil, nil, fmt.Errorf("%s command must not specify both --nmea and --binary options", cmdName)
		}
		if rtcmMsg.IsSet() || pvtMsg.IsSet() || rawMsg.IsSet() {
			return nil, nil, fmt.Errorf("%s command must not specify --nmea together with --rtcm-out, --pvt-out or --raw-out options", cmdName)
		}
		if nmeaMsg.Get()&gpsprot.NMEAMsgAny == 0 {
			nmeaMsg.Set(gpsprot.NMEAMsgRMC)
		}
		rtcmMsg.Set(gpsprot.RTCMMsgNone)
		pvtMsg.Set(0)
		rawMsg.Set(gpsprot.RawMsgNone)
		// we aren't exposing satsMsg in the CLI yet (waiting to implement UBX-CFG-SIGNAL)
		vars.satsMsg.Set(gpsprot.SatsMsgNone)
	} else if binary {
		configChanged = true
		if nmeaMsg.IsSet() {
			return nil, nil, fmt.Errorf("%s command must not specify both --nmea-out and --binary options", cmdName)
		} else {
			nmeaMsg.Set(gpsprot.NMEAMsgNone)
		}
		// ensure we have some non-NMEA output
		if rtcmMsg.Get()&gpsprot.RTCMMsgAny == 0 && pvtMsg.Get()&gpsprot.PVTMsgAny == 0 {
			pvtMsg.Set(gpsprot.PVTMsgPos | gpsprot.PVTMsgTime) // analogous to NMEA RMC
		}
	}
	vars.nmeaMsg = nmeaMsg
	vars.rtcmMsg = rtcmMsg
	vars.pvtMsg = pvtMsg
	vars.rawMsg = rawMsg
	if vars.remoteSpeed != 0 || vars.pps || vars.primaryGNSS != 0 {
		configChanged = true
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

type rawOutOpt gpsprot.Option[gpsprot.RawMsgFlags]

var _ pflag.Value = (*rawOutOpt)(nil)

func (rawOut *rawOutOpt) String() string {
	msg := (*gpsprot.Option[gpsprot.RawMsgFlags])(rawOut)
	if !msg.IsSet() {
		return ""
	}
	flags := msg.Get()
	if flags == 0 {
		return "none"
	}
	var parts []string
	if flags&gpsprot.RawMsgObs != 0 {
		parts = append(parts, "obs")
	}
	if flags&gpsprot.RawMsgNavData != 0 {
		parts = append(parts, "nav")
	}
	return strings.Join(parts, ",")
}

func (rawOut *rawOutOpt) Type() string {
	return "raw-flags"
}

func (rawOut *rawOutOpt) Set(s string) error {
	if s == "" {
		return fmt.Errorf("empty value for --raw-out")
	}
	msg := (*gpsprot.Option[gpsprot.RawMsgFlags])(rawOut)
	flags := gpsprot.RawMsgFlags(0)
	for _, w := range strings.Split(s, ",") {
		switch strings.ToLower(strings.TrimSpace(w)) {
		case "obs":
			flags |= gpsprot.RawMsgObs
		case "nav":
			flags |= gpsprot.RawMsgNavData
		case "none":
			// do nothing
		default:
			return fmt.Errorf("unknown raw output flag: %s", w)
		}
	}
	msg.Set(flags)
	return nil
}

type pvtOutOpt gpsprot.PVTMsgFlags

var _ pflag.Value = (*pvtOutOpt)(nil)

func (pvtOut *pvtOutOpt) String() string {
	flags := gpsprot.PVTMsgFlags(*pvtOut)
	if !flags.IsSet() {
		return ""
	}
	var parts []string
	if flags&gpsprot.PVTMsgPos != 0 {
		parts = append(parts, "pos")
	}
	if flags&gpsprot.PVTMsgVel != 0 {
		parts = append(parts, "vel")
	}
	if flags&gpsprot.PVTMsgTime != 0 {
		parts = append(parts, "time")
	}
	if flags&gpsprot.PVTMsgTimePulse != 0 {
		parts = append(parts, "tp")
	}
	if flags&gpsprot.PVTMsgLeapSecond != 0 {
		parts = append(parts, "leap")
	}
	if flags&gpsprot.PVTMsgTAI != 0 {
		parts = append(parts, "tai")
	}
	if flags&gpsprot.PVTMsgECEF != 0 {
		parts = append(parts, "ecef")
	}
	if flags&gpsprot.PVTMsgOff != 0 {
		parts = append(parts, "off")
	}
	return strings.Join(parts, ",")
}

func (pvtOut *pvtOutOpt) Type() string {
	return "pvt-flags"
}

func (pvtOut *pvtOutOpt) Set(s string) error {
	if s == "" {
		return fmt.Errorf("empty value for --pvt-out")
	}
	flags := gpsprot.PVTMsgFlags(*pvtOut)
	for _, w := range strings.Split(s, ",") {
		switch strings.ToLower(strings.TrimSpace(w)) {
		case "pos":
			flags |= gpsprot.PVTMsgPos
		case "vel":
			flags |= gpsprot.PVTMsgVel
		case "time":
			flags |= gpsprot.PVTMsgTime
		case "tp":
			flags |= gpsprot.PVTMsgTimePulse
		case "leap":
			flags |= gpsprot.PVTMsgLeapSecond
		case "tai":
			flags |= gpsprot.PVTMsgTAI
		case "ecef":
			flags |= gpsprot.PVTMsgECEF
		case "off":
			flags |= gpsprot.PVTMsgOff
		case "daemon":
			flags |= gpsevent.PVTMsgFlags
		default:
			return fmt.Errorf("unknown pvt output flag: %s", w)
		}
	}
	*pvtOut = pvtOutOpt(flags)
	return nil
}

type rtcmOutOpt gpsprot.Option[gpsprot.RTCMMsgFlags]

var _ pflag.Value = (*rtcmOutOpt)(nil)

func (rtcmOut *rtcmOutOpt) String() string {
	msg := (*gpsprot.Option[gpsprot.RTCMMsgFlags])(rtcmOut)
	if !msg.IsSet() {
		return ""
	}
	flags := msg.Get()
	if flags == 0 {
		return "none"
	}
	var parts []string
	if flags&gpsprot.RTCMMsgMSM4 != 0 {
		parts = append(parts, "MSM4")
	}
	if flags&gpsprot.RTCMMsgMSM7 != 0 {
		parts = append(parts, "MSM7")
	}
	if flags&gpsprot.RTCMMsgARP != 0 {
		parts = append(parts, "ARP")
	}
	// Note: We don't expose RTCMMsgOther as per requirement
	return strings.Join(parts, ",")
}

func (rtcmOut *rtcmOutOpt) Type() string {
	return "rtcm-flags"
}

func (rtcmOut *rtcmOutOpt) Set(s string) error {
	if s == "" {
		return fmt.Errorf("empty value for --rtcm-out")
	}
	msg := (*gpsprot.Option[gpsprot.RTCMMsgFlags])(rtcmOut)
	flags := gpsprot.RTCMMsgFlags(0)
	for _, w := range strings.Split(s, ",") {
		switch strings.ToUpper(strings.TrimSpace(w)) {
		case "MSM4":
			flags |= gpsprot.RTCMMsgMSM4
		case "MSM7":
			flags |= gpsprot.RTCMMsgMSM7
		case "ARP":
			flags |= gpsprot.RTCMMsgARP
		case "NONE":
			// do nothing
		default:
			return fmt.Errorf("unknown rtcm output flag: %s", w)
		}
	}
	msg.Set(flags)
	return nil
}

type nmeaOutOpt gpsprot.Option[gpsprot.NMEAMsgFlags]

var _ pflag.Value = (*nmeaOutOpt)(nil)

func (nmeaOut *nmeaOutOpt) String() string {
	msg := (*gpsprot.Option[gpsprot.NMEAMsgFlags])(nmeaOut)
	if !msg.IsSet() {
		return ""
	}
	flags := msg.Get()
	if flags == 0 {
		return "none"
	}
	var parts []string
	if flags&gpsprot.NMEAMsgRMC != 0 {
		parts = append(parts, "RMC")
	}
	if flags&gpsprot.NMEAMsgGGA != 0 {
		parts = append(parts, "GGA")
	}
	if flags&gpsprot.NMEAMsgGSA != 0 {
		parts = append(parts, "GSA")
	}
	if flags&gpsprot.NMEAMsgGSV != 0 {
		parts = append(parts, "GSV")
	}
	if flags&gpsprot.NMEAMsgZDA != 0 {
		parts = append(parts, "ZDA")
	}
	// Don't expose NMEAMsgOther
	return strings.Join(parts, ",")
}

func (nmeaOut *nmeaOutOpt) Type() string {
	return "nmea-flags"
}

func (nmeaOut *nmeaOutOpt) Set(s string) error {
	if s == "" {
		return fmt.Errorf("empty value for --nmea-out")
	}
	msg := (*gpsprot.Option[gpsprot.NMEAMsgFlags])(nmeaOut)
	flags := gpsprot.NMEAMsgFlags(0)
	for _, w := range strings.Split(s, ",") {
		switch strings.ToUpper(strings.TrimSpace(w)) {
		case "RMC":
			flags |= gpsprot.NMEAMsgRMC
		case "GGA":
			flags |= gpsprot.NMEAMsgGGA
		case "GSA":
			flags |= gpsprot.NMEAMsgGSA
		case "GSV":
			flags |= gpsprot.NMEAMsgGSV
		case "ZDA":
			flags |= gpsprot.NMEAMsgZDA
		case "NONE":
			// do nothing
		default:
			return fmt.Errorf("unknown nmea output flag: %s", w)
		}
	}
	msg.Set(flags)
	return nil
}
