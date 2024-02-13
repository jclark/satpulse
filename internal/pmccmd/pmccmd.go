package pmccmd

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"

	"github.com/jclark/satpulse/internal/cmd"
	"github.com/jclark/satpulse/internal/pmc"
	"github.com/spf13/pflag"
)

type hexUint8 uint8

var _ pflag.Value = (*hexUint8)(nil)

func (u *hexUint8) Set(s string) error {
	v, err := strconv.ParseUint(s, 0, 8)
	if err != nil {
		return err
	}
	*u = hexUint8(v)
	return nil
}

func (u *hexUint8) String() string {
	return fmt.Sprintf("0x%02x", uint8(*u))
}

func (*hexUint8) Type() string {
	return "uint8"
}

func hexUint8Flag(flags *pflag.FlagSet, name string, value hexUint8, usage string) *hexUint8 {
	flags.Var(&value, name, usage)
	return &value
}

func Cmd(lg *slog.Logger, progName string, cmdName string, args []string) (usage string, err error) {
	msg, usageFunc, err := createMsg(cmdName, args)
	if msg == nil {
		if usageFunc != nil {
			usage = usageFunc(progName)
		}
		return
	}
	cfg := pmc.NewClientConfig()
	cfg.SetDefaults()
	response, err := sendRecv(cfg, msg)
	if err != nil {
		return
	}
	if response != nil {
		gsm, ok := response.(*pmc.GrandmasterSettingsMsg)
		if ok {
			printGrandmasterSettings(os.Stdout, gsm.V.Data)
		} else {
			fmt.Printf("Response: %+v\n", response.Value())
		}
	}
	return
}

func sendRecv(cfg *pmc.ClientConfig, msg pmc.MgmtMsg) (pmc.MgmtMsg, error) {
	client, err := pmc.NewClient(cfg)
	if err != nil {
		return nil, err
	}
	msg, err = client.SendRecv(msg)
	if err != nil {
		return nil, err
	}
	err = client.Close()
	return msg, err
}

const summary = `[-h|--help] [-s|--set] [-m|--management-id ID]
     [--utc-offset N] [--clock-class N] [--clock-accuracy N] [--offset-scaled-log-variance N] [--time-source N]
	 [--leap61] [--leap59] [--utc-offset-valid] [--ptp-timescale] [--time-traceable] [--frequency-traceable]`

func createMsg(cmdName string, args []string) (pmc.MgmtMsg, func(string) string, error) {
	flags := pflag.NewFlagSet(cmdName, pflag.ContinueOnError)
	help := false
	flags.BoolVarP(&help, "help", "h", false, "show help")

	set := flags.BoolP("set", "s", false, "set the grandmaster settings")
	utcOffset := flags.Int("utc-offset", 37, "TAI-UTC offset in seconds")
	clockClass := hexUint8Flag(flags, "clock-class", 0x6, "clock class")
	clockAccuracy := hexUint8Flag(flags, "clock-acccuracy", 0x23, "clock accuracy")
	offsetScaledLogVariance := flags.Uint("offset-scaled-log-variance", 0xFFFF, "offset scaled log variance")
	timeSource := hexUint8Flag(flags, "time-source", 0x20, "time source")
	timeBoolFlags := map[string]*struct {
		flag  pmc.TimeFlags
		usage string
		set   bool
	}{
		"leap61":              {usage: "positive leap second at end of current UTC day", flag: pmc.Leap61},
		"leap59":              {usage: "negative leap second at end of current UTC day", flag: pmc.Leap59},
		"utc-offset-valid":    {usage: "current UTC offset is traceable", flag: pmc.CurrentUTCOffsetValid},
		"ptp-timescale":       {usage: "use PTP timescale", flag: pmc.PTPTimescale},
		"time-traceable":      {usage: "time is traceable", flag: pmc.TimeTraceable},
		"frequency-traceable": {usage: "frequency is traceable", flag: pmc.FrequencyTraceable},
	}
	for name, f := range timeBoolFlags {
		flags.BoolVar(&f.set, name, false, f.usage)
	}
	midFlag := flags.StringP("management-id", "m", "GRANDMASTER_SETTINGS_NP", "management id to get")
	usage := cmd.UsageFunc(cmdName, summary, flags)
	err := flags.Parse(args)
	if err != nil {
		return nil, usage, err
	}
	if help {
		return nil, usage, nil
	}
	mid, err := pmc.ParseMID(*midFlag)
	if err != nil {
		return nil, nil, err
	}
	if !*set {
		return pmc.NewMgmtGetMsg(pmc.MgmtID(mid)), nil, nil
	}
	if mid != pmc.MIDGrandmasterSettings {
		return nil, nil, errors.New("only GRANDMASTER_SETTINGS_NP can be set")
	}
	var gs pmc.GrandmasterSettings
	gs.UTCOffset = int16(*utcOffset)
	gs.ClockQuality.ClockClass = uint8(*clockClass)
	gs.ClockQuality.ClockAccuracy = pmc.ClockAccuracy(*clockAccuracy)
	gs.ClockQuality.OffsetScaledLogVariance = uint16(*offsetScaledLogVariance)
	gs.TimeSource = pmc.TimeSource(uint8(*timeSource))
	for _, f := range timeBoolFlags {
		if f.set {
			gs.TimeFlags |= f.flag
		}
	}
	return pmc.NewMgmtSetMsg(gs), nil, nil
}

func printGrandmasterSettings(f *os.File, gs pmc.GrandmasterSettings) {
	fmt.Fprint(f, "managementId: GRANDMASTER_SETTINGS_NP\n")
	fmt.Fprintf(f, "clockClass: 0x%02x\n", gs.ClockQuality.ClockClass)
	acc := gs.ClockQuality.ClockAccuracy
	fmt.Fprintf(f, "clockAccuracy: %s (%s)\n", acc.String(), acc.Description())
	fmt.Fprintf(f, "offsetScaledLogVariance: 0x%04x\n", gs.ClockQuality.OffsetScaledLogVariance)
	fmt.Fprintf(f, "timeSource: %s (%s)\n", gs.TimeSource.String(), gs.TimeSource.Description())
	fmt.Fprintf(f, "utcOffset: %d\n", gs.UTCOffset)
	fmt.Fprintf(f, "utcOffsetValid: %v\n", gs.TimeFlags&pmc.CurrentUTCOffsetValid != 0)
	fmt.Fprintf(f, "ptpTimescale: %v\n", gs.TimeFlags&pmc.PTPTimescale != 0)
	fmt.Fprintf(f, "timeTraceable: %v\n", gs.TimeFlags&pmc.TimeTraceable != 0)
	fmt.Fprintf(f, "frequencyTraceable: %v\n", gs.TimeFlags&pmc.FrequencyTraceable != 0)
	fmt.Fprintf(f, "leap61: %v\n", gs.TimeFlags&pmc.Leap61 != 0)
	fmt.Fprintf(f, "leap59: %v\n", gs.TimeFlags&pmc.Leap59 != 0)
}
