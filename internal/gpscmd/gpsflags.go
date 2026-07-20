package gpscmd

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/jclark/satpulse/gps/app/cmd"
	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/gpsreg"
	"github.com/jclark/satpulse/gps/lib/geopos"
	"github.com/jclark/satpulse/gps/lib/opt"
	"github.com/jclark/satpulse/gps/lib/term"
	"github.com/jclark/satpulse/gps/ptime"
	"github.com/pelletier/go-toml/v2"
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
	configFile     string
	packetLogPath  string
	packetLogMode  packetLogMode
	capture        opt.Val[time.Duration]
	timeGNSS       gpsprot.GNSS
	navMsgAuth     opt.Val[gpsprot.NavMsgAuth]
	enabledSignals gpsprot.SignalSet
	pps            opt.Val[time.Duration]
	antCableDelay  opt.Val[time.Duration]
	minElev        opt.Val[gpsprot.Angle]
	rtcmBaseID     opt.Val[uint16]
	baudRate       opt.Val[uint32]
	mode           opt.Val[gpsprot.Mode]
	configOpts     gpsprot.ConfigOptions
	configSupport  configSupportReq
	configGet      gpsprot.PropIDs
	targetJSON     string
	msgFilePath    string
	msgTags        []string
	msgSave        bool
	msgPort        string
	vendor         gpsreg.Vendor
	showReceiver   bool
	showTags       bool
	jsonOut        bool
}

type configSupportReq struct {
	all       gpsprot.ConfigSupportFlags
	options   map[gpsprot.ConfigSupportFlags][]string
	msmOption string
}

func (r configSupportReq) isZero() bool {
	return r.all == 0 && r.msmOption == ""
}

func (r *configSupportReq) require(flags gpsprot.ConfigSupportFlags, option string) {
	r.init()
	r.all |= flags
	for flag := gpsprot.ConfigSupportFlags(1); flag <= gpsprot.ConfigSupportFull; flag <<= 1 {
		if flags&flag != 0 {
			r.options[flag] = append(r.options[flag], option)
		}
	}
}

func (r *configSupportReq) requireMSM(option string) {
	r.msmOption = option
}

func (r *configSupportReq) init() {
	if r.options == nil {
		r.options = make(map[gpsprot.ConfigSupportFlags][]string)
	}
}

func (r configSupportReq) flags() (gpsprot.ConfigSupportFlags, gpsprot.ConfigSupportFlags) {
	var msm gpsprot.ConfigSupportFlags
	if r.msmOption != "" {
		msm = gpsprot.ConfigSupportRTCMMSM
	}
	return r.all, msm
}

func (r configSupportReq) unsupportedOptions(supported gpsprot.ConfigSupportFlags) []string {
	var opts []string
	seen := make(map[string]bool)
	for flag := gpsprot.ConfigSupportFlags(1); flag <= gpsprot.ConfigSupportFull; flag <<= 1 {
		if r.all&flag == 0 || supported&flag != 0 {
			continue
		}
		for _, opt := range r.options[flag] {
			if opt != "" && !seen[opt] {
				opts = append(opts, opt)
				seen[opt] = true
			}
		}
	}
	if r.msmOption != "" && supported&gpsprot.ConfigSupportRTCMMSM == 0 && !seen[r.msmOption] {
		opts = append(opts, r.msmOption)
	}
	sort.Strings(opts)
	return opts
}

const summary = `[-h|--help] [-d|--serial-device path] [-s|--device-speed bps] [-f|--config-file path]
       	    [--socket path] [--packet-log path] [--capture seconds] [--save] [--speed bps] [--nmea] [--binary]
            [-c|--show-config] [--show-port] [--json] [--save] [--save-all] [--reset] [--reload] [--factory-reset]
            [-g|--gnss GPS|GAL|BDS|GLO|QZSS|NAVIC|SBAS,...] [-b|--band L1|L2|L5|E5|L6,...]
            [--signal signal,...] [--except-signal signal,...]
            [-p|--pps width] [--ant-cable-delay nanos] [--time-gnss GPS|GAL|BDS|GLO]
            [--mobile] [--fixed-pos-ecef x,y,z] [--fixed-pos-llh lat,lon,height] [--fixed-pos-acc meters]
            [--survey] [--survey-time seconds] [--survey-acc meters]
            [--min-elev degrees] [--vendor name] [--rtcm-base-id id]
            [--pvt-out pos|vel|time|tp|leap|survey|qual|epoch|tai|ecef|ptp|ntp|off,...]
            [--sats-out sat|sig|none,...] [--rtcm-out MSM4|MSM7|ARP|auto|none,...]
            [--raw-out obs|nav|none,...] [--nmea-out RMC|GGA|GSA|GSV|ZDA|VTG|GLL|none,...]
            [-m|--msg-file path] [-t|--tag name,...] [--port name] [--show-tags]`

const defaultSurveyTime = 2000
const defaultSurveyAcc = 20 * gpsprot.Meter
const defaultFixedPosAcc = 20 * gpsprot.Meter

// TODO: add PropIDBaudRate to showProps so --show-config reports the current configured speed.
// Would need regeneration of the *-noop replay-test traces.
const showProps = gpsprot.PropIDSignalsEnabled |
	gpsprot.PropIDMode |
	gpsprot.PropIDTimePulse |
	gpsprot.PropIDTimeGNSS |
	gpsprot.PropIDAntennaCableDelay |
	gpsprot.PropIDMinElevation |
	gpsprot.PropIDRTCMBaseID

type flagParser struct {
	vars  flagVars
	flags *pflag.FlagSet

	help           bool
	gl             gnssList
	bands          bands
	timeGNSS       majorGNSS
	save           bool
	saveAll        bool
	reset          bool
	factoryReset   bool
	reload         bool
	showConfig     bool
	showPort       bool
	testLogPath    string
	nmea           bool
	binary         bool
	survey         bool
	surveyTime     uint32
	surveyAcc      length
	sysTimeTrusted bool
	osnma          bool
	fixedPosECEF   ecef
	fixedPosLLH    llh
	fixedPosAcc    length
	pps            float64
	antCableDelay  int64
	mobile         bool
	capture        float64
	rawOut         rawOutOpt
	pvtOut         pvtOutOpt
	rtcmOut        rtcmOutOpt
	nmeaOut        nmeaOutOpt
	satsOut        satsOutOpt
	msgTags        string
	vendorStr      string
	baudRate       uint32
	addSignals     signalList
	exceptSignals  signalList
	minElev        angle
	rtcmBaseID     uint16
}

func parseFlags(cmdName string, args []string) (*flagVars, func(string) string, error) {
	p := newFlagParser(cmdName)
	usage := cmd.UsageFunc(cmdName, summary, p.flags)
	err := p.flags.Parse(args)
	if err != nil {
		return nil, usage, err
	}
	if p.help {
		return nil, usage, nil
	}
	if p.flags.NArg() != 0 {
		return nil, usage, fmt.Errorf("%s command must not have non-option arguments", cmdName)
	}
	return p.resolve(cmdName, usage)
}

func newFlagParser(cmdName string) *flagParser {
	p := &flagParser{}
	p.bands = bands(gpsprot.BandAll)
	p.surveyTime = uint32(defaultSurveyTime)
	p.surveyAcc = length(defaultSurveyAcc)
	p.fixedPosAcc = length(defaultFixedPosAcc)
	p.flags = pflag.NewFlagSet(cmdName, pflag.ContinueOnError)
	flags := p.flags
	vars := &p.vars

	flags.BoolVarP(&p.help, "help", "h", false, "show help")
	flags.BoolVarP(&p.showConfig, "show-config", "c", false, "show current GPS receiver configuration")
	flags.BoolVar(&p.showPort, "show-port", false, "show the receiver port the host is communicating on and its serial speed when applicable")
	flags.BoolVar(&vars.jsonOut, "json", false, "write receiver information and configuration results to stdout as a single JSON object")
	flags.BoolVar(&p.save, "save", false, "save configuration changes to non-volatile memory on the GPS receiver")
	flags.BoolVar(&p.saveAll, "save-all", false, "save the current configuration to non-volatile memory on the GPS receiver")
	flags.BoolVar(&p.reset, "reset", false, "reset the GPS receiver and perform a cold start")
	flags.BoolVar(&p.reload, "reload", false, "reload the GPS receiver configuration from non-volatile memory")
	flags.BoolVar(&p.factoryReset, "factory-reset", false, "reset the GPS receiver to factory defaults")
	flags.BoolVar(&p.nmea, "nmea", false, "enable NMEA output and disable binary output from the GPS receiver")
	flags.BoolVar(&p.binary, "binary", false, "enable binary output and disable NMEA output from the GPS receiver")
	flags.BoolVar(&vars.showReceiver, "show-receiver", false, "detect and display GPS receiver information")
	flags.StringVarP(&vars.serialDevice, "serial-device", "d", "", "serial device connected to GPS receiver")
	flags.StringVar(&vars.socketPath, "socket", "", "`path` of socket to connect to GPS receiver")
	flags.StringVarP(&vars.configFile, "config-file", "f", "", "`path` to satpulse TOML configuration file")
	flags.StringVar(&vars.targetJSON, "target-json", "", "JSON configuration target or - to read from stdin")
	flags.MarkHidden("target-json")
	flags.StringVar(&vars.packetLogPath, "packet-log", "", "log packets to `path`")
	flags.StringVarP(&vars.msgFilePath, "msg-file", "m", "", "`path` to TOML file containing message definitions")
	flags.StringVarP(&p.msgTags, "tag", "t", "", "comma-separated `list` of tags to send (in order)")
	flags.BoolVar(&vars.showTags, "show-tags", false, "list all tags in the message file with descriptions, then exit")
	flags.StringVar(&vars.msgPort, "port", "", "u-blox receiver `port` for port-dependent message-file entries: i2c|uart1|uart2|usb|spi")
	flags.StringVar(&p.vendorStr, "vendor", "", "GPS receiver `vendor` name")
	flags.Float64Var(&p.capture, "capture", 0, "capture packets for `seconds` after config (0 = forever)")
	flags.StringVar(&p.testLogPath, "test-log", "", "log test data to `path`")
	flags.MarkHidden("test-log")
	flags.IntVarP(&vars.localSpeed, "device-speed", "s", 0, "serial device baud-rate in `bps`")
	flags.Uint32Var(&p.baudRate, "speed", 0, "set GPS receiver baud-rate in `bps`")
	flags.VarP(&p.gl, "gnss", "g", "enabled GNSS constellations `list`: GPS|GAL|BDS|GLO|QZSS|NAVIC|SBAS,...")
	flags.VarP(&p.bands, "band", "b", "enabled GNSS bands `list`: L1,L2,L5,E5,E6,...")
	flags.Var(&p.addSignals, "signal", "enabled GNSS signals `list`: GPSL1|E1|B1C,...")
	flags.Var(&p.exceptSignals, "except-signal", "excluded GNSS signals `list`: GPSL1C|E5b,...")
	flags.Var(&p.timeGNSS, "time-gnss", "GNSS `constellation` used for timing: GPS|GAL|BDS|GLO")
	flags.Var(&p.rawOut, "raw-out", "raw data messages to output `flags`: obs|nav|none,...")
	flags.Var(&p.pvtOut, "pvt-out", "PVT messages to output `flags`: pos|vel|time|tp|leap|survey|qual|epoch|tai|ecef|after|ptp|ntp|off,...")
	flags.Var(&p.rtcmOut, "rtcm-out", "RTCM messages to output `flags`: MSM4|MSM7|ARP|auto|none,...")
	flags.Var(&p.nmeaOut, "nmea-out", "NMEA messages to output `flags`: RMC|GGA|GSA|GSV|ZDA|VTG|GLL|none,...")
	flags.Var(&p.satsOut, "sats-out", "satellite data messages to output `flags`: sat|sig|none,...")
	flags.Float64VarP(&p.pps, "pps", "p", 0, "configure the GPS receiver to enable a PPS signal with pulse `width` in seconds")
	flags.Int64Var(&p.antCableDelay, "ant-cable-delay", 0, "antenna cable delay in nanoseconds")
	flags.BoolVar(&p.mobile, "mobile", false, "the GPS receiver is not stationary; disable time mode")
	flags.BoolVar(&p.survey, "survey", false, "instruct the GPS receiver to perform a survey")
	flags.Uint32Var(&p.surveyTime, "survey-time", defaultSurveyTime, "survey time in seconds")
	flags.Var(&p.surveyAcc, "survey-acc", "survey accuracy in `meters`")
	flags.Var(&p.fixedPosECEF, "fixed-pos-ecef", "fixed ECEF position as `x,y,z` in meters")
	flags.Var(&p.fixedPosLLH, "fixed-pos-llh", "fixed LLH position as `lat,lon,height` in degrees and meters")
	flags.Var(&p.fixedPosAcc, "fixed-pos-acc", "accuracy of fixed position in `meters`")
	flags.Var(&p.minElev, "min-elev", "minimum satellite elevation in `degrees`")
	flags.Uint16Var(&p.rtcmBaseID, "rtcm-base-id", 0, "RTCM reference station `id` (0-4095)")
	flags.BoolVar(&vars.configOpts.SetStatic, "static", false, "make the receiver use static positioning mode if it is not already doing so")
	flags.MarkHidden("static")
	flags.BoolVar(&p.sysTimeTrusted, "sys-time-trusted", false, "provide system time as trusted time to the GPS receiver")
	flags.MarkHidden("sys-time-trusted")
	flags.BoolVar(&p.osnma, "osnma", false, "enable OSNMA authentication for Galileo")
	flags.MarkHidden("osnma")
	return p
}

func (p *flagParser) resolve(cmdName string, usage func(string) string) (*flagVars, func(string) string, error) {
	vars := &p.vars
	if vars.targetJSON != "" {
		allowed := map[string]bool{
			"target-json": true, "serial-device": true, "device-speed": true, "socket": true, "config-file": true,
			"packet-log": true, "test-log": true, "capture": true, "vendor": true, "json": true,
			"show-receiver": true, "show-config": true, "show-port": true,
		}
		var bad string
		p.flags.Visit(func(f *pflag.Flag) {
			if bad == "" && !allowed[f.Name] {
				bad = f.Name
			}
		})
		if bad != "" {
			return nil, nil, fmt.Errorf("--target-json cannot be combined with --%s", bad)
		}
	}
	if vars.jsonOut && vars.msgFilePath != "" {
		return nil, nil, fmt.Errorf("--json cannot be combined with --msg-file")
	}
	if vars.showTags {
		if vars.msgFilePath == "" {
			return nil, usage, fmt.Errorf("--show-tags requires --msg-file")
		}
		// --show-tags doesn't require --socket or --serial-device
		return vars, nil, nil
	}
	configChanged, u, err := p.resolveConn(cmdName, usage)
	if err != nil {
		return nil, u, err
	}
	if vars.targetJSON != "" {
		// JSON targets bypass flag-layer configuration policy. Display
		// requests retain their capability requirements.
		p.resolveShow()
		return vars, nil, nil
	}
	changed, err := p.resolveSignals(cmdName)
	if err != nil {
		return nil, nil, err
	}
	configChanged = configChanged || changed
	changed, err = p.resolveMsgOut(cmdName)
	if err != nil {
		return nil, nil, err
	}
	configChanged = configChanged || changed
	changed, err = p.resolveMode(cmdName)
	if err != nil {
		return nil, nil, err
	}
	configChanged = configChanged || changed
	changed, err = p.resolveTimePulse()
	if err != nil {
		return nil, nil, err
	}
	configChanged = configChanged || changed
	changed, err = p.resolveMisc()
	if err != nil {
		return nil, nil, err
	}
	configChanged = configChanged || changed
	p.resolveShow()
	if err := p.resolveSave(cmdName, configChanged); err != nil {
		return nil, nil, err
	}
	doConfigure, err := p.resolveConfigure(cmdName, configChanged)
	if err != nil {
		return nil, nil, err
	}
	if err := p.resolveMsgFile(configChanged, doConfigure); err != nil {
		return nil, nil, err
	}
	return vars, nil, nil
}

func (p *flagParser) resolveConn(cmdName string, usage func(string) string) (bool, func(string) string, error) {
	vars := &p.vars
	flags := p.flags
	configChanged := false
	if p.vendorStr != "" {
		v, err := gpsreg.ParseVendor(p.vendorStr)
		if err != nil {
			return false, nil, err
		}
		vars.vendor = v
	}
	if vars.configFile != "" {
		if vars.serialDevice != "" || vars.localSpeed != 0 {
			return false, usage, fmt.Errorf("--config-file cannot be combined with --serial-device or --device-speed")
		}
		if err := loadConfigFile(vars); err != nil {
			return false, nil, err
		}
	}
	if (vars.socketPath == "") == (vars.serialDevice == "") {
		return false, usage, fmt.Errorf("%s command must specify either --socket or --serial-device or --config-file", cmdName)
	}
	if p.testLogPath != "" {
		if vars.packetLogPath != "" {
			return false, usage, fmt.Errorf("%s command must not specify both --packet-log and --test-log", cmdName)
		}
		vars.packetLogPath = p.testLogPath
		vars.packetLogMode = testLogMode
	}
	if flags.Lookup("capture").Changed {
		if p.capture < 0 {
			return false, usage, fmt.Errorf("--capture duration must not be negative")
		}
		if vars.packetLogPath == "" && vars.msgFilePath == "" {
			return false, usage, fmt.Errorf("--capture requires --packet-log")
		}
		vars.capture.Set(ptime.Seconds(p.capture))
	}

	for _, s := range []string{"device-speed", "speed"} {
		f := flags.Lookup(s)
		if f.Changed && f.Value.String() == "0" {
			return false, usage, fmt.Errorf("0 is not a valid value for --%s", s)
		}
	}

	if p.baudRate != 0 {
		configChanged = true
		if !term.IsValidSpeed(int(p.baudRate)) {
			return false, nil, fmt.Errorf("invalid remote serial speed %d", p.baudRate)
		}
		vars.baudRate.Set(p.baudRate)
		vars.configSupport.require(gpsprot.ConfigSupportSpeed, "--speed")
	}
	return configChanged, nil, nil
}

func (p *flagParser) resolveSignals(cmdName string) (bool, error) {
	vars := &p.vars
	flags := p.flags
	configChanged := false
	addSigs := gpsprot.SignalSet(p.addSignals)
	exceptSigs := gpsprot.SignalSet(p.exceptSignals)
	if len(p.gl.gnss) == 0 {
		if flags.Lookup("band").Changed {
			return false, fmt.Errorf("%s command must specify --gnss when --band is specified", cmdName)
		}
		if exceptSigs != 0 {
			return false, fmt.Errorf("%s command must specify --gnss when --except-signal is specified", cmdName)
		}
	}
	if both := addSigs & exceptSigs; both != 0 {
		return false, fmt.Errorf("signals in both --signal and --except-signal: %s", both)
	}
	if len(p.gl.gnss) != 0 || addSigs != 0 {
		vars.enabledSignals = (gpsprot.Band(p.bands).SignalSet(p.gl.gnss...) | addSigs) &^ exceptSigs
		if (vars.enabledSignals&gpsprot.SigSetMajor)&^gpsprot.SigSetAugment == 0 {
			return false, fmt.Errorf("at least one non-augmentation signal from a major GNSS must be enabled")
		}
		configChanged = true
		if flags.Lookup("band").Changed {
			vars.configSupport.require(gpsprot.ConfigSupportSignal, "--band")
		}
		if addSigs != 0 {
			vars.configSupport.require(gpsprot.ConfigSupportSignal, "--signal")
		}
		if exceptSigs != 0 {
			vars.configSupport.require(gpsprot.ConfigSupportSignal, "--except-signal")
		}
	}
	return configChanged, nil
}

func (p *flagParser) resolveMsgOut(cmdName string) (bool, error) {
	vars := &p.vars
	configChanged := false
	pvtMsg := gpsprot.PVTMsgFlags(p.pvtOut)
	rawMsg := (opt.Val[gpsprot.RawMsgFlags])(p.rawOut)
	rtcmMsg := (opt.Val[gpsprot.RTCMMsgFlags])(p.rtcmOut)
	nmeaMsg := (opt.Val[gpsprot.NMEAMsgFlags])(p.nmeaOut)
	satsMsg := (opt.Val[gpsprot.SatsMsgFlags])(p.satsOut)

	if rawMsg.IsSet() || pvtMsg.IsSet() || rtcmMsg.IsSet() || nmeaMsg.IsSet() || satsMsg.IsSet() {
		configChanged = true
	}
	addMsgConfigSupport(&vars.configSupport, vars.enabledSignals, rawMsg, pvtMsg, rtcmMsg)

	if p.nmea {
		configChanged = true
		// the concept of --nmea is to give NMEA output only
		// we require that there is some NMEA output
		// --nmea-out can be used to specify what; defaults to RMC
		if p.binary {
			return false, fmt.Errorf("%s command must not specify both --nmea and --binary options", cmdName)
		}
		if rtcmMsg.IsSet() || pvtMsg.IsSet() || rawMsg.IsSet() || satsMsg.IsSet() {
			return false, fmt.Errorf("%s command must not specify --nmea together with --rtcm-out, --pvt-out, --raw-out or --sats-out options", cmdName)
		}
		if nmeaMsg.Get()&gpsprot.NMEAMsgAny == 0 {
			nmeaMsg.Set(gpsprot.NMEAMsgRMC)
		}
		rtcmMsg.Set(gpsprot.RTCMMsgNone)
		pvtMsg.Set(gpsprot.PVTMsgOff)
		rawMsg.Set(gpsprot.RawMsgNone)
		satsMsg.Set(gpsprot.SatsMsgNone)
	} else if p.binary {
		configChanged = true
		if nmeaMsg.IsSet() {
			return false, fmt.Errorf("%s command must not specify both --nmea-out and --binary options", cmdName)
		} else {
			nmeaMsg.Set(gpsprot.NMEAMsgNone)
		}
		// ensure we have some non-NMEA output
		if rtcmMsg.Get()&gpsprot.RTCMMsgAny == 0 && !pvtMsg.IsSet() {
			pvtMsg.Set(gpsprot.PVTMsgPos | gpsprot.PVTMsgTime) // analogous to NMEA RMC
		}
	}
	vars.configOpts.NMEAMsg = nmeaMsg
	vars.configOpts.RTCMMsg = rtcmMsg
	vars.configOpts.PVTMsg = pvtMsg
	vars.configOpts.RawMsg = rawMsg
	vars.configOpts.SatsMsg = satsMsg
	return configChanged, nil
}

func (p *flagParser) resolveMode(cmdName string) (bool, error) {
	vars := &p.vars
	flags := p.flags
	configChanged := false
	if p.survey || vars.configOpts.SetStatic {
		configChanged = true
		vars.configOpts.Survey.MinDur = time.Duration(p.surveyTime) * time.Second
		if gpsprot.Length(p.surveyAcc) < gpsprot.Millimeter {
			return false, fmt.Errorf("--survey-acc must be at least 0.001 (1 mm)")
		}
		vars.configOpts.Survey.AccLimit = gpsprot.Length(p.surveyAcc)
		if p.survey {
			vars.configSupport.require(gpsprot.ConfigSupportSurvey, "--survey")
			if flags.Lookup("survey-acc").Changed {
				vars.configSupport.require(gpsprot.ConfigSupportSurveyAcc, "--survey-acc")
			}
			vars.configOpts.Survey.Flags |= gpsprot.SurveyAgain
			vars.mode.Set(gpsprot.Mode{Static: true})
			if p.mobile {
				return false, fmt.Errorf("%s command must not specify both --mobile and --survey", cmdName)
			}
		}
	} else if p.mobile {
		configChanged = true
		vars.mode.Set(gpsprot.Mode{Static: false})
	}
	if !gpsprot.Point3D(p.fixedPosECEF).IsZero() {
		configChanged = true
		if err := p.resolveFixedPos(cmdName, "--fixed-pos-ecef", gpsprot.Mode{
			PosType:      gpsprot.PosTypeECEF,
			FixedPosECEF: gpsprot.Point3D(p.fixedPosECEF),
		}); err != nil {
			return false, err
		}
	}
	if flags.Lookup("fixed-pos-llh").Changed {
		if flags.Lookup("fixed-pos-ecef").Changed {
			return false, fmt.Errorf("%s command must not specify both --fixed-pos-ecef and --fixed-pos-llh", cmdName)
		}
		configChanged = true
		if err := p.resolveFixedPos(cmdName, "--fixed-pos-llh", gpsprot.Mode{
			PosType:     gpsprot.PosTypeLLH,
			FixedPosLLH: p.fixedPosLLH.latLon,
			Height:      p.fixedPosLLH.height,
		}); err != nil {
			return false, err
		}
	}
	return configChanged, nil
}

func (p *flagParser) resolveFixedPos(cmdName, flagName string, mode gpsprot.Mode) error {
	vars := &p.vars
	if gpsprot.Length(p.fixedPosAcc) < gpsprot.Millimeter {
		return fmt.Errorf("--fixed-pos-acc must be at least 0.001 (1 mm)")
	}
	vars.configSupport.require(gpsprot.ConfigSupportFixedPos, flagName)
	if p.flags.Lookup("fixed-pos-acc").Changed {
		vars.configSupport.require(gpsprot.ConfigSupportFixedPosAcc, "--fixed-pos-acc")
	}
	mode.Static = true
	mode.FixedPosAcc = gpsprot.Length(p.fixedPosAcc)
	vars.mode.Set(mode)
	if p.mobile {
		return fmt.Errorf("%s command must not specify both --mobile and %s", cmdName, flagName)
	} else if p.survey {
		return fmt.Errorf("%s command must not specify both --survey and %s", cmdName, flagName)
	}
	return nil
}

func (p *flagParser) resolveTimePulse() (bool, error) {
	vars := &p.vars
	flags := p.flags
	configChanged := false
	vars.timeGNSS = gpsprot.GNSS(p.timeGNSS)
	if vars.timeGNSS != 0 && !vars.enabledSignals.IsZero() {
		if vars.enabledSignals.GNSSSet()&gpsprot.GNSSSetOf(vars.timeGNSS) == 0 {
			return false, fmt.Errorf("%s specified as --time-gnss but none of its signals are enabled", vars.timeGNSS)
		}
	}
	if vars.timeGNSS != 0 {
		configChanged = true
	}
	if flags.Lookup("pps").Changed {
		if p.pps < 0 || p.pps >= 1 {
			return false, fmt.Errorf("--pps pulse width must be >= 0 and < 1 second")
		}
		vars.pps.Set(time.Duration(p.pps * float64(time.Second)))
		configChanged = true
	}
	if flags.Lookup("ant-cable-delay").Changed {
		vars.antCableDelay.Set(time.Duration(p.antCableDelay))
		configChanged = true
	}
	return configChanged, nil
}

func (p *flagParser) resolveMisc() (bool, error) {
	vars := &p.vars
	flags := p.flags
	configChanged := false
	if flags.Lookup("min-elev").Changed {
		elev := gpsprot.Angle(p.minElev)
		if elev < -90*gpsprot.Degrees || elev > 90*gpsprot.Degrees {
			return false, fmt.Errorf("--min-elev must be between -90 and 90 degrees")
		}
		vars.minElev.Set(elev)
		configChanged = true
	}
	if flags.Lookup("rtcm-base-id").Changed {
		if p.rtcmBaseID > 4095 {
			return false, fmt.Errorf("--rtcm-base-id must be between 0 and 4095")
		}
		vars.rtcmBaseID.Set(p.rtcmBaseID)
		vars.configSupport.require(gpsprot.ConfigSupportRTCMBaseID, "--rtcm-base-id")
		configChanged = true
	}
	if p.sysTimeTrusted {
		est, err := EstimateSystemTime()
		if err != nil {
			return false, fmt.Errorf("failed to estimate system time: %w", err)
		}
		vars.configOpts.TimeAssist = *est
		vars.configOpts.TimeAssist.Trusted = true
	}
	if flags.Lookup("osnma").Changed {
		configChanged = true
		nma := gpsprot.NavMsgAuthNone
		if p.osnma {
			nma |= gpsprot.NavMsgAuthOSNMA
			copy(vars.configOpts.OSNMA.MerkleTreeRoot[:], OSNMAMerkleTreeRoot)
		}
		vars.navMsgAuth.Set(nma)
	}
	return configChanged, nil
}

func (p *flagParser) resolveShow() {
	vars := &p.vars
	if p.showConfig {
		vars.configGet = showProps
	}
	if p.showPort {
		vars.configGet |= gpsprot.PropIDPort | gpsprot.PropIDBaudRate
		vars.configSupport.require(gpsprot.ConfigSupportPort, "--show-port")
	}
}

func (p *flagParser) resolveSave(cmdName string, configChanged bool) error {
	vars := &p.vars
	if p.save {
		if p.saveAll {
			return fmt.Errorf("%s command must not specify both --save and --save-all", cmdName)
		}
		if !configChanged && vars.msgFilePath == "" {
			return fmt.Errorf("no configuration changes and no message file: nothing for --save to do; use --save-all to save current configuration")
		}
		// In message-file mode --save is a plain boolean for VALSET layers,
		// not a configurator save; don't populate ConfigOptions.Save.
		if vars.msgFilePath == "" {
			vars.configOpts.Save = gpsprot.SaveMinimal
		}
	} else if p.saveAll {
		vars.configOpts.Save = gpsprot.SaveAll
	}
	return nil
}

func (p *flagParser) resolveConfigure(cmdName string, configChanged bool) (bool, error) {
	vars := &p.vars
	// tests whether configurator needs to run
	doConfigure := p.save || p.saveAll || p.reset || p.reload || p.showConfig || p.showPort || configChanged
	if p.factoryReset {
		if doConfigure {
			return false, fmt.Errorf("%s command must not use --factory-reset with --save, --save-all, --reset, --reload, --show-config, --show-port or configuration changes", cmdName)
		}
		doConfigure = true
		vars.configOpts.Reset = gpsprot.ResetFactory
	} else if p.reset || p.reload {
		if configChanged && !p.save && !p.saveAll {
			return false, fmt.Errorf("--reset or --reload without saving would lose configuration changes")
		}
		if p.showConfig || p.showPort {
			return false, fmt.Errorf("cannot use --reset or --reload with --show-config or --show-port")
		}
		if p.reset && p.reload {
			return false, fmt.Errorf("cannot use both --reset and --reload")
		}
		if p.reload {
			vars.configOpts.Reset = gpsprot.ResetReload
		} else {
			vars.configOpts.Reset = gpsprot.ResetCold
		}
		doConfigure = true
	} else if vars.showReceiver {
		doConfigure = true
	}
	return doConfigure, nil
}

func (p *flagParser) resolveMsgFile(configChanged, doConfigure bool) error {
	vars := &p.vars
	flags := p.flags
	if vars.msgFilePath != "" {
		// In message-file mode --save is permitted (it controls VALSET
		// layers for [[ubxval]]) but no other configuration flag is.
		if p.saveAll || p.reset || p.reload || p.factoryReset || p.showConfig || p.showPort || configChanged || vars.showReceiver {
			return fmt.Errorf("--msg-file cannot be combined with configuration flags")
		}
		if vars.packetLogMode == testLogMode {
			return fmt.Errorf("--msg-file cannot be combined with --test-log")
		}
		if flags.Lookup("port").Changed {
			port, err := normalizeUBXPort(vars.msgPort)
			if err != nil {
				return err
			}
			vars.msgPort = port
		}
		vars.msgSave = p.save
		// Parse tags: split on comma, preserving empty strings for empty tags
		vars.msgTags = strings.Split(p.msgTags, ",")
	} else {
		if p.msgTags != "" {
			return fmt.Errorf("--tag requires --msg-file")
		}
		if flags.Lookup("port").Changed {
			return fmt.Errorf("--port requires --msg-file")
		}
		if !doConfigure && !vars.capture.IsSet() {
			vars.showReceiver = true
		}
		if vars.jsonOut && !doConfigure && !vars.showReceiver {
			return fmt.Errorf("--json cannot be used with passive packet capture")
		}
	}
	return nil
}

// normalizeUBXPort lowercases s and verifies it is one of the u-blox port
// names. Used for early flag-level validation.
func normalizeUBXPort(s string) (string, error) {
	p := strings.ToLower(s)
	switch p {
	case "i2c", "uart1", "uart2", "usb", "spi":
		return p, nil
	}
	return "", fmt.Errorf("invalid --port value %q: must be one of i2c, uart1, uart2, usb, spi", s)
}

func addMsgConfigSupport(req *configSupportReq, sigs gpsprot.SignalSet, rawMsg opt.Val[gpsprot.RawMsgFlags], pvtMsg gpsprot.PVTMsgFlags, rtcmMsg opt.Val[gpsprot.RTCMMsgFlags]) {
	if rawMsg.IsSet() && rawMsg.Get()&gpsprot.RawMsgAny != 0 {
		req.require(gpsprot.ConfigSupportRaw, "--raw-out")
	}
	if pvtMsg&gpsprot.PVTMsgSurvey != 0 {
		req.require(gpsprot.ConfigSupportSurveyMsg, "--pvt-out")
	}
	if !rtcmMsg.IsSet() {
		return
	}
	rtcm := rtcmMsg.Get()
	msm := rtcm & (gpsprot.RTCMMsgMSM4 | gpsprot.RTCMMsgMSM7)
	if rtcm&gpsprot.RTCMMsgLax != 0 {
		if msm != 0 {
			req.requireMSM("--rtcm-out")
		}
	} else {
		if msm&gpsprot.RTCMMsgMSM4 != 0 {
			req.require(gpsprot.ConfigSupportRTCMMSM4, "--rtcm-out")
		}
		if msm&gpsprot.RTCMMsgMSM7 != 0 {
			req.require(gpsprot.ConfigSupportRTCMMSM7, "--rtcm-out")
		}
	}
	if rtcm&gpsprot.RTCMMsgARP != 0 && msm == 0 {
		req.requireMSM("--rtcm-out")
	}
	if msm != 0 && sigs.GNSSSet().Contains(gpsprot.QZSS) {
		req.require(gpsprot.ConfigSupportRTCMQZSS, "--rtcm-out")
	}
}

// loadConfigFile reads serial device and speed from a satpulse TOML config file.
func loadConfigFile(v *flagVars) error {
	var cfg struct {
		Serial struct {
			Device string `toml:"device"`
			Speed  *int   `toml:"speed"`
		} `toml:"serial"`
	}
	f, err := os.Open(v.configFile)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := toml.NewDecoder(f).Decode(&cfg); err != nil {
		return fmt.Errorf("%s: %w", v.configFile, err)
	}
	if cfg.Serial.Device == "" {
		return fmt.Errorf("%s: serial.device is not set", v.configFile)
	}
	if cfg.Serial.Speed == nil {
		return fmt.Errorf("%s: serial.speed is not set", v.configFile)
	}
	v.serialDevice = cfg.Serial.Device
	v.localSpeed = *cfg.Serial.Speed
	return nil
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
	for w := range strings.SplitSeq(s, ",") {
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

func (bp *bands) String() string {
	b := gpsprot.Band(*bp)
	if b == 0 {
		return "(none)"
	}
	if b == gpsprot.BandAll {
		return "all"
	}
	return strings.Join(b.Items(), ",")
}

func (bp *bands) Type() string {
	return "bands"
}

func (bp *bands) Set(s string) error {
	var b gpsprot.Band
	for w := range strings.SplitSeq(s, ",") {
		w = strings.Trim(w, " \t")
		band, ok := gpsprot.ParseBandName(w)
		if !ok {
			return fmt.Errorf("unknown band: %s", w)
		}
		b |= band
	}
	*bp = bands(b)
	return nil
}

type signalList gpsprot.SignalSet

var _ pflag.Value = (*signalList)(nil)

func (sl *signalList) String() string {
	var s []string
	for sig := range gpsprot.SignalSet(*sl).Signals() {
		s = append(s, sig.String())
	}
	return strings.Join(s, ",")
}

func (sl *signalList) Type() string {
	return "signal-list"
}

func (sl *signalList) Set(s string) error {
	ss := gpsprot.SignalSet(*sl)
	for w := range strings.SplitSeq(s, ",") {
		sig, err := gpsprot.ParseSignal(strings.Trim(w, " \t"))
		if err != nil {
			return err
		}
		ss |= gpsprot.SignalSetOf(sig)
	}
	*sl = signalList(ss)
	return nil
}

type rawOutOpt opt.Val[gpsprot.RawMsgFlags]

var _ pflag.Value = (*rawOutOpt)(nil)

func (rawOut *rawOutOpt) String() string {
	msg := (*opt.Val[gpsprot.RawMsgFlags])(rawOut)
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
	msg := (*opt.Val[gpsprot.RawMsgFlags])(rawOut)
	flags := gpsprot.RawMsgFlags(0)
	for w := range strings.SplitSeq(s, ",") {
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
	if flags&gpsprot.PVTMsgQuality != 0 {
		parts = append(parts, "qual")
	}
	if flags&gpsprot.PVTMsgEpoch != 0 {
		parts = append(parts, "epoch")
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
	for w := range strings.SplitSeq(s, ",") {
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
		case "qual":
			flags |= gpsprot.PVTMsgQuality
		case "epoch":
			flags |= gpsprot.PVTMsgEpoch
		case "tai":
			flags |= gpsprot.PVTMsgTAI
		case "ecef":
			flags |= gpsprot.PVTMsgECEF
		case "after":
			flags |= gpsprot.PVTMsgTimePulseAfter
		case "off":
			flags |= gpsprot.PVTMsgOff
		case "ptp":
			flags |= gpsprot.PVTMsgTimingPTP | gpsprot.PVTMsgOff
		case "ntp":
			flags |= gpsprot.PVTMsgTimingSerialUTC | gpsprot.PVTMsgOff
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

type rtcmOutOpt opt.Val[gpsprot.RTCMMsgFlags]

var _ pflag.Value = (*rtcmOutOpt)(nil)

func (rtcmOut *rtcmOutOpt) String() string {
	msg := (*opt.Val[gpsprot.RTCMMsgFlags])(rtcmOut)
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
	msg := (*opt.Val[gpsprot.RTCMMsgFlags])(rtcmOut)
	flags := gpsprot.RTCMMsgFlags(0)
	auto := false
	for w := range strings.SplitSeq(s, ",") {
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

type nmeaOutOpt opt.Val[gpsprot.NMEAMsgFlags]

var _ pflag.Value = (*nmeaOutOpt)(nil)

func (nmeaOut *nmeaOutOpt) String() string {
	msg := (*opt.Val[gpsprot.NMEAMsgFlags])(nmeaOut)
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
	if flags&gpsprot.NMEAMsgGLL != 0 {
		parts = append(parts, "GLL")
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
	msg := (*opt.Val[gpsprot.NMEAMsgFlags])(nmeaOut)
	flags := gpsprot.NMEAMsgFlags(0)
	for w := range strings.SplitSeq(s, ",") {
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
		case "GLL":
			flags |= gpsprot.NMEAMsgGLL
		case "NONE":
			// do nothing
		default:
			return fmt.Errorf("unknown nmea output flag: %s", w)
		}
	}
	msg.Set(flags)
	return nil
}

type satsOutOpt opt.Val[gpsprot.SatsMsgFlags]

var _ pflag.Value = (*satsOutOpt)(nil)

func (satsOut *satsOutOpt) String() string {
	msg := (*opt.Val[gpsprot.SatsMsgFlags])(satsOut)
	if !msg.IsSet() {
		return ""
	}
	flags := msg.Get()
	if flags == 0 {
		return "none"
	}
	var parts []string
	if flags&gpsprot.SatsMsgSat != 0 {
		parts = append(parts, "sat")
	}
	if flags&gpsprot.SatsMsgSignal != 0 {
		parts = append(parts, "sig")
	}
	return strings.Join(parts, ",")
}

func (satsOut *satsOutOpt) Type() string {
	return "sats-flags"
}

func (satsOut *satsOutOpt) Set(s string) error {
	if s == "" {
		return fmt.Errorf("empty value for --sats-out")
	}
	msg := (*opt.Val[gpsprot.SatsMsgFlags])(satsOut)
	flags := gpsprot.SatsMsgFlags(0)
	for w := range strings.SplitSeq(s, ",") {
		switch strings.ToLower(strings.TrimSpace(w)) {
		case "sat":
			flags |= gpsprot.SatsMsgSat
		case "sig":
			flags |= gpsprot.SatsMsgSignal
		case "none":
			// do nothing
		default:
			return fmt.Errorf("unknown satellite output flag: %s", w)
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

type llh struct {
	latLon [2]gpsprot.Angle
	height gpsprot.Length
}

var _ pflag.Value = (*llh)(nil)

func (l *llh) String() string {
	if l.latLon[0] == 0 && l.latLon[1] == 0 && l.height == 0 {
		return ""
	}
	return l.latLon[0].String() + "," + l.latLon[1].String() + "," + l.height.String()
}

func (l *llh) Type() string {
	return "lat,lon,height"
}

func (l *llh) Set(s string) error {
	parts := strings.Split(s, ",")
	if len(parts) != 3 {
		return fmt.Errorf("invalid LLH coordinates: %q", s)
	}
	lat, err := gpsprot.ParseAngle(strings.TrimSpace(parts[0]))
	if err != nil {
		return fmt.Errorf("invalid LLH coordinates: %q", s)
	}
	lon, err := gpsprot.ParseAngle(strings.TrimSpace(parts[1]))
	if err != nil {
		return fmt.Errorf("invalid LLH coordinates: %q", s)
	}
	height, err := gpsprot.ParseLength(strings.TrimSpace(parts[2]))
	if err != nil {
		return fmt.Errorf("invalid LLH coordinates: %q", s)
	}
	if lat < -90*gpsprot.Degrees || lat > 90*gpsprot.Degrees {
		return fmt.Errorf("invalid LLH coordinates: latitude %v out of range [-90, 90]", lat)
	}
	if lon < -180*gpsprot.Degrees || lon > 180*gpsprot.Degrees {
		return fmt.Errorf("invalid LLH coordinates: longitude %v out of range [-180, 180]", lon)
	}
	if height < -500*gpsprot.Meter || height > 10000*gpsprot.Meter {
		return fmt.Errorf("invalid LLH coordinates: height %v out of range [-500, 10000] meters", height)
	}
	l.latLon[0] = lat
	l.latLon[1] = lon
	l.height = height
	return nil
}

type length gpsprot.Length

var _ pflag.Value = (*length)(nil)

func (l *length) String() string {
	return gpsprot.Length(*l).String()
}

func (l *length) Type() string {
	return "meters"
}

func (l *length) Set(s string) error {
	v, err := gpsprot.ParseLength(s)
	if err != nil {
		return err
	}
	*l = length(v)
	return nil
}

type angle gpsprot.Angle

var _ pflag.Value = (*angle)(nil)

func (a *angle) String() string {
	return gpsprot.Angle(*a).String()
}

func (a *angle) Type() string {
	return "degrees"
}

func (a *angle) Set(s string) error {
	v, err := gpsprot.ParseAngle(s)
	if err != nil {
		return err
	}
	*a = angle(v)
	return nil
}
