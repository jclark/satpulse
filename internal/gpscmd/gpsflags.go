package gpscmd

import (
	"fmt"
	"strings"
	"time"

	"github.com/jclark/satpulse/internal/cmd"
	"github.com/jclark/satpulse/internal/geopos"
	"github.com/jclark/satpulse/internal/gpsevent"
	"github.com/jclark/satpulse/internal/gpsprot"
	"github.com/jclark/satpulse/term"
	"github.com/spf13/pflag"
)

type packetLogMode int

const OSNMAMerkleTreeRoot = "\x83\x2E\x15\xED\xE5\x56\x55\xEA\xC6\xE3\x99\xA5\x39\x47\x7B\x7C\x03\x4C\xCE\x24\xC3\xC9\x3F\xFC\x90\x4A\xCD\x9B\xF8\x42\xF0\x4E"

const (
	packetsOnlyLogMode packetLogMode = iota
	testLogMode
)

type flagVars struct {
	localSpeed     int
	serialDevice   string
	socketPath     string
	packetLogPath  string
	packetLogMode  packetLogMode
	timeGNSS       gpsprot.GNSS
	navMsgAuth     gpsprot.Option[gpsprot.NavMsgAuth]
	enabledSignals gpsprot.SignalSet
	pps            gpsprot.Option[time.Duration]
	antCableDelay  gpsprot.Option[time.Duration]
	mode           gpsprot.Option[gpsprot.Mode]
	configOpts     gpsprot.ConfigOptions
	configGet      gpsprot.PropIDs
}

const summary = `[-h|--help] [-d|--serial-device path] [-s|--device-speed bps] [--force-probe]
       	    [--socket path] [--packet-log path] [--save] [--speed bps] [--nmea] [--binary]
            [-c|--show-config] [--save] [--save-all] [--reset] [--reload] [--factory-reset]
            [-g|--gnss GPS|GAL|BDS|GLO|QZSS|NAVIC|SBAS,...] [-b|--band L1|L2|L5|E5|L6,...]
            [-p|--pps width] [--ant-cable-delay nanos] [--time-gnss GPS|GAL|BDS|GLO]
            [--mobile] [--fixed-pos-ecef x,y,z] [--fixed-pos-acc meters]
            [--survey] [--survey-time seconds] [--survey-acc meters]
            [--raw-out obs|nav|none,...] [--pvt-out pos|vel|time|tp|leap|survey|tai|ecef|off,...]
            [--rtcm-out MSM4|MSM7|ARP|auto|none,...] [--nmea-out RMC|GGA|GSA|GSV|ZDA|VTG|none,...]`

const defaultSurveyTime = 2000
const defaultSurveyAcc = 20.0
const defaultFixedPosAcc = 20.0
const showProps = gpsprot.PropIDSignalsEnabled |
	gpsprot.PropIDMode |
	gpsprot.PropIDTimePulse |
	gpsprot.PropIDTimeGNSS |
	gpsprot.PropIDAntennaCableDelay

func parseFlags(cmdName string, args []string) (*flagVars, func(string) string, error) {
	help := false
	vars := flagVars{}
	gl := gnssList{}
	bands := bands(gpsprot.BandAll)
	var timeGNSS majorGNSS

	save := false
	saveAll := false
	reset := false
	factoryReset := false
	reload := false
	showConfig := false
	testLogPath := ""
	nmea := false
	binary := false
	survey := false
	surveyTime := uint32(defaultSurveyTime)
	surveyAcc := defaultSurveyAcc
	sysTimeTrusted := false
	osnma := false
	forceProbe := false
	var fixedPosECEF ecef
	fixedPosAcc := defaultFixedPosAcc
	pps := 0.0
	antCableDelay := int64(0)
	mobile := false

	var rawOut rawOutOpt
	var pvtOut pvtOutOpt
	var rtcmOut rtcmOutOpt
	var nmeaOut nmeaOutOpt

	flags := pflag.NewFlagSet(cmdName, pflag.ContinueOnError)

	flags.BoolVarP(&help, "help", "h", false, "show help")
	flags.BoolVarP(&showConfig, "show-config", "c", false, "show current GPS receiver configuration")
	flags.BoolVar(&save, "save", false, "save configuration changes to non-volatile memory on the GPS receiver")
	flags.BoolVar(&saveAll, "save-all", false, "save the current configuration to non-volatile memory on the GPS receiver")
	flags.BoolVar(&reset, "reset", false, "reset the GPS receiver and perform a cold start")
	flags.BoolVar(&reload, "reload", false, "reload the GPS receiver configuration from non-volatile memory")
	flags.BoolVar(&factoryReset, "factory-reset", false, "reset the GPS receiver to factory defaults")
	flags.BoolVar(&nmea, "nmea", false, "enable NMEA output and disable binary output from the GPS receiver")
	flags.BoolVar(&binary, "binary", false, "enable binary output and disable NMEA output from the GPS receiver")
	flags.BoolVar(&forceProbe, "force-probe", false, "force writing probe to serial device even if when no output from GPS receiver")
	flags.StringVarP(&vars.serialDevice, "serial-device", "d", "", "serial device connected to GPS receiver")
	flags.StringVar(&vars.socketPath, "socket", "", "`path` of socket to connect to GPS receiver")
	flags.StringVar(&vars.packetLogPath, "packet-log", "", "log packets to `path`")
	flags.StringVar(&testLogPath, "test-log", "", "log test data to `path`")
	flags.MarkHidden("test-log")
	flags.IntVarP(&vars.localSpeed, "device-speed", "s", 0, "serial device baud-rate in `bps`")
	flags.Uint32Var(&vars.configOpts.BaudRate, "speed", 0, "set GPS receiver baud-rate in `bps`")
	flags.VarP(&gl, "gnss", "g", "enabled GNSS constellations `list`: GPS|GAL|BDS|GLO|QZSS|NAVIC|SBAS,...")
	flags.VarP(&bands, "band", "b", "enabled GNSS bands `list`: L1,L2,L5,E5,E6,...")
	flags.Var(&timeGNSS, "time-gnss", "GNSS `constellation` used for timing: GPS|GAL|BDS|GLO")
	flags.Var(&rawOut, "raw-out", "raw data messages to output `flags`: obs|nav|none,...")
	flags.Var(&pvtOut, "pvt-out", "PVT messages to output `flags`: pos|vel|time|tp|leap|survey|tai|ecef|after|daemon|off,...")
	flags.Var(&rtcmOut, "rtcm-out", "RTCM messages to output `flags`: MSM4|MSM7|ARP|auto|none,...")
	flags.Var(&nmeaOut, "nmea-out", "NMEA messages to output `flags`: RMC|GGA|GSA|GSV|ZDA|VTG|none,...")
	flags.Float64VarP(&pps, "pps", "p", 0, "configure the GPS receiver to enable a PPS signal with pulse `width` in seconds")
	flags.Int64Var(&antCableDelay, "ant-cable-delay", 0, "antenna cable delay in nanoseconds")
	flags.BoolVar(&mobile, "mobile", false, "the GPS receiver is not stationary; disable time mode")
	flags.BoolVar(&survey, "survey", false, "instruct the GPS receiver to perform a survey")
	flags.Uint32Var(&surveyTime, "survey-time", defaultSurveyTime, "survey time in seconds")
	flags.Float64Var(&surveyAcc, "survey-acc", defaultSurveyAcc, "survey accuracy in meters")
	flags.Var(&fixedPosECEF, "fixed-pos-ecef", "fixed ECEF position as `x,y,z` in meters")
	flags.Float64Var(&fixedPosAcc, "fixed-pos-acc", defaultFixedPosAcc, "accuracy of fixed position in meters")
	flags.BoolVar(&vars.configOpts.SetStatic, "static", false, "make the receiver use static positioning mode if it is not already doing so")
	flags.MarkHidden("static")
	flags.BoolVar(&sysTimeTrusted, "sys-time-trusted", false, "provide system time as trusted time to the GPS receiver")
	flags.MarkHidden("sys-time-trusted")
	flags.BoolVar(&osnma, "osnma", false, "enable OSNMA authentication for Galileo")
	flags.MarkHidden("osnma")
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

	for _, s := range []string{"device-speed", "speed"} {
		f := flags.Lookup(s)
		if f.Changed && f.Value.String() == "0" {
			return nil, usage, fmt.Errorf("0 is not a valid value for --%s", s)
		}
	}
	configChanged := false

	if vars.configOpts.BaudRate != 0 {
		configChanged = true
		if !term.IsValidSpeed(int(vars.configOpts.BaudRate)) {
			return nil, nil, fmt.Errorf("invalid remote serial speed %d", vars.configOpts.BaudRate)
		}
	}

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
		pvtMsg.Set(gpsprot.PVTMsgOff)
		rawMsg.Set(gpsprot.RawMsgNone)
		// we aren't exposing satsMsg in the CLI yet (waiting to implement UBX-CFG-SIGNAL)
		vars.configOpts.SatsMsg.Set(gpsprot.SatsMsgNone)
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
	vars.configOpts.NMEAMsg = nmeaMsg
	vars.configOpts.RTCMMsg = rtcmMsg
	vars.configOpts.PVTMsg = pvtMsg
	vars.configOpts.RawMsg = rawMsg

	if survey || vars.configOpts.SetStatic {
		configChanged = true
		vars.configOpts.Survey.MinDur = time.Duration(surveyTime) * time.Second
		if surveyAcc < 0.001 {
			return nil, nil, fmt.Errorf("--survey-acc must at least 0.001 (1 mm)")
		}
		vars.configOpts.Survey.AccLimit = gpsprot.Meters(surveyAcc)
		if survey {
			vars.configOpts.Survey.Flags |= gpsprot.SurveyAgain
			vars.mode.Set(gpsprot.Mode{Static: true})
			if mobile {
				return nil, nil, fmt.Errorf("%s command must not specify both --mobile and --survey", cmdName)
			}
		}
	} else if mobile {
		configChanged = true
		vars.mode.Set(gpsprot.Mode{Static: false})
	}
	if !gpsprot.Point3D(fixedPosECEF).IsZero() {
		configChanged = true
		if fixedPosAcc < 0.001 {
			return nil, nil, fmt.Errorf("--fixed-pos-acc must be at least 0.001 (1 mm)")
		}
		vars.mode.Set(gpsprot.Mode{
			Static:       true,
			PosType:      gpsprot.PosTypeECEF,
			FixedPosECEF: gpsprot.Point3D(fixedPosECEF),
			FixedPosAcc:  gpsprot.Meters(fixedPosAcc),
		})
		if mobile {
			return nil, nil, fmt.Errorf("%s command must not specify both --mobile and --fixed-pos-ecef", cmdName)
		} else if survey {
			return nil, nil, fmt.Errorf("%s command must not specify both --survey and --fixed-pos-ecef", cmdName)
		}
	}
	vars.timeGNSS = gpsprot.GNSS(timeGNSS)
	if vars.timeGNSS != 0 && len(gl.gnss) != 0 {
		if vars.enabledSignals.GNSSSet()&gpsprot.GNSSSetOf(vars.timeGNSS) == 0 {
			return nil, nil, fmt.Errorf("%s specified as --time-gnss but not included in --gnss", vars.timeGNSS)
		}
	}
	if flags.Lookup("pps").Changed {
		if pps < 0 || pps >= 1 {
			return nil, nil, fmt.Errorf("--pps pulse width must be >= 0 and < 1 second")
		}
		vars.pps.Set(time.Duration(pps * float64(time.Second)))
		configChanged = true
	}
	if flags.Lookup("ant-cable-delay").Changed {
		vars.antCableDelay.Set(time.Duration(antCableDelay))
		configChanged = true
	}
	if vars.timeGNSS != 0 {
		configChanged = true
	}
	if sysTimeTrusted {
		est, err := EstimateSystemTime()
		if err != nil {
			return nil, nil, fmt.Errorf("failed to estimate system time: %w", err)
		}
		vars.configOpts.TimeAssist = *est
		vars.configOpts.TimeAssist.Trusted = true
	}
	if flags.Lookup("osnma").Changed {
		configChanged = true
		nma := gpsprot.NavMsgAuthNone
		if osnma {
			nma |= gpsprot.NavMsgAuthOSNMA
			copy(vars.configOpts.OSNMA.MerkleTreeRoot[:], OSNMAMerkleTreeRoot)
		}
		vars.navMsgAuth.Set(nma)
	}
	if showConfig {
		vars.configGet = showProps
	}
	if forceProbe {
		vars.configOpts.ForceProbe = gpsprot.ForceProbeWhenNoOutput
	}
	if save {
		if !configChanged {
			return nil, nil, fmt.Errorf("no configuration changes to save with --save; use --save-all to save current configuration")
		}
		if saveAll {
			return nil, nil, fmt.Errorf("%s command must not specify both --save and --save-all", cmdName)
		}
		vars.configOpts.Save = gpsprot.SaveMinimal
	} else if saveAll {
		vars.configOpts.Save = gpsprot.SaveAll
	}
	if factoryReset {
		if save || saveAll || reset || reload || showConfig || configChanged {
			return nil, nil, fmt.Errorf("%s command must not use --factory-reset with --save, --save-all, --reset, --reload, --show-config or configuration changes", cmdName)
		}
		vars.configOpts.Reset = gpsprot.ResetFactory
	} else if reset || reload {
		if configChanged && !save && !saveAll {
			return nil, nil, fmt.Errorf("--reset or --reload without saving would lose configuration changes")
		}
		if showConfig {
			return nil, nil, fmt.Errorf("cannot use --reset or --reload with --show-config")
		}
		if reset && reload {
			return nil, nil, fmt.Errorf("cannot use both --reset and --reload")
		}
		if reload {
			vars.configOpts.Reset = gpsprot.ResetReload
		} else {
			vars.configOpts.Reset = gpsprot.ResetCold
		}
	}
	return &vars, nil, nil
}

type majorGNSS gpsprot.GNSS

var _ pflag.Value = (*majorGNSS)(nil)

func (mg *majorGNSS) String() string {
	if *mg == 0 {
		// this stops it printing "GNSS(0)" as the default value in help
		return ""
	}
	return gpsprot.GNSS(*mg).String()
}

func (mg *majorGNSS) Type() string {
	return "major-gnss"
}

func (mg *majorGNSS) Set(s string) error {
	gnss, err := gpsprot.ParseGNSS(s)
	if err != nil {
		return err
	}
	if !gnss.IsMajor() {
		return fmt.Errorf("%s: not a major GNSS constellation; use GPS, GAL, BDS, or GLO", s)
	}
	*mg = majorGNSS(gnss)
	return nil
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
	{"E5", gpsprot.BandL5 | gpsprot.BandE5b}, // needs to be before L5 and E5b for String() to work
	{"L5", gpsprot.BandL5},
	{"E5b", gpsprot.BandE5b},
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
	if flags&gpsprot.PVTMsgSurvey != 0 {
		parts = append(parts, "survey")
	}
	if flags&gpsprot.PVTMsgTAI != 0 {
		parts = append(parts, "tai")
	}
	if flags&gpsprot.PVTMsgECEF != 0 {
		parts = append(parts, "ecef")
	}
	if flags&gpsprot.PVTMsgTimePulseAfter != 0 {
		parts = append(parts, "after")
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
		case "survey":
			flags |= gpsprot.PVTMsgSurvey
		case "tai":
			flags |= gpsprot.PVTMsgTAI
		case "ecef":
			flags |= gpsprot.PVTMsgECEF
		case "after":
			flags |= gpsprot.PVTMsgTimePulseAfter
		case "off":
			flags |= gpsprot.PVTMsgOff
		case "daemon":
			flags |= gpsevent.TimePulsePVTMsgFlags | gpsprot.PVTMsgOff
		default:
			return fmt.Errorf("unknown pvt output flag: %s", w)
		}
	}
	// Validate PVT flag combinations
	// "after" requires "tp"
	if flags&gpsprot.PVTMsgTimePulseAfter != 0 && flags&gpsprot.PVTMsgTimePulse == 0 {
		return fmt.Errorf("'after' requires 'tp'")
	}
	// "tai" requires "after" or "time"
	if flags&gpsprot.PVTMsgTAI != 0 && flags&(gpsprot.PVTMsgTimePulse|gpsprot.PVTMsgTime) == 0 {
		return fmt.Errorf("'tai' requires 'tp' or 'time'")
	}
	// "ecef" requires "pos" or "vel"
	if flags&gpsprot.PVTMsgECEF != 0 && flags&(gpsprot.PVTMsgPos|gpsprot.PVTMsgVel) == 0 {
		return fmt.Errorf("'ecef' requires 'pos' or 'vel'")
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
	auto := flags&gpsprot.RTCMMsgLax != 0
	if !auto && flags&gpsprot.RTCMMsgMSM4 != 0 {
		parts = append(parts, "MSM4")
	}
	if flags&gpsprot.RTCMMsgMSM7 != 0 {
		parts = append(parts, "MSM7")
	}
	if !auto && flags&gpsprot.RTCMMsgARP != 0 {
		parts = append(parts, "ARP")
	}
	if auto {
		parts = append(parts, "auto")
	}
	// We don't expose RTCMMsgOther
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
	auto := false
	for _, w := range strings.Split(s, ",") {
		switch strings.ToUpper(strings.TrimSpace(w)) {
		case "MSM4":
			flags |= gpsprot.RTCMMsgMSM4
		case "MSM7":
			flags |= gpsprot.RTCMMsgMSM7
		case "ARP":
			flags |= gpsprot.RTCMMsgARP
		case "AUTO":
			auto = true
		case "NONE":
			// do nothing
		default:
			return fmt.Errorf("unknown rtcm output flag: %s", w)
		}
	}
	if flags&(gpsprot.RTCMMsgMSM4|gpsprot.RTCMMsgMSM7) == (gpsprot.RTCMMsgMSM4 | gpsprot.RTCMMsgMSM7) {
		return fmt.Errorf("MSM4 and MSM7 cannot be used together")
	}
	if auto {
		flags |= gpsprot.RTCMMsgARP | gpsprot.RTCMMsgLax
		if flags&gpsprot.RTCMMsgMSM7 == 0 {
			flags |= gpsprot.RTCMMsgMSM4
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
	if flags&gpsprot.NMEAMsgVTG != 0 {
		parts = append(parts, "VTG")
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
		case "VTG":
			flags |= gpsprot.NMEAMsgVTG
		case "NONE":
			// do nothing
		default:
			return fmt.Errorf("unknown nmea output flag: %s", w)
		}
	}
	msg.Set(flags)
	return nil
}

type ecef gpsprot.Point3D

var _ pflag.Value = (*ecef)(nil)

func (e *ecef) String() string {
	if e[0] == 0 && e[1] == 0 && e[2] == 0 {
		return ""
	}
	return (*gpsprot.Point3D)(e).String()
}

func (e *ecef) Type() string {
	return "x,y,z"
}

func (e *ecef) Set(s string) error {
	pt, err := gpsprot.ParsePoint3D(s)
	if err != nil {
		return err
	}
	// Validate the coordinates are on Earth
	ecefCoords := geopos.ECEF{
		pt[0].Meters(),
		pt[1].Meters(),
		pt[2].Meters(),
	}
	if err := ecefCoords.CheckOnEarth(); err != nil {
		return fmt.Errorf("invalid ECEF coordinates: %w", err)
	}
	*e = ecef(pt)
	return nil
}
