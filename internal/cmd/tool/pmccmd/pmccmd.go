package pmccmd

import (
	"fmt"
	"log/slog"
	"strconv"

	"github.com/jclark/satpulse/internal/cmd/tool"
	"github.com/jclark/satpulse/internal/pmc"
	"github.com/spf13/pflag"
)

type hexUint8 int

func (i *hexUint8) Set(s string) error {
	v, err := strconv.ParseUint(s, 0, 8)
	if err != nil {
		return err
	}
	*i = hexUint8(v)
	return nil
}

func (i *hexUint8) String() string {
	return fmt.Sprintf("0x%02x", uint8(*i))
}

func (i *hexUint8) Type() string {
	return "int"
}

func hexUint8Flag(flags *pflag.FlagSet, name string, value hexUint8, usage string) *hexUint8 {
	flags.Var(&value, name, usage)
	return &value
}

func Cmd(lg *slog.Logger, progName string, cmdName string, args []string) (usage string, err error) {
	msg, usageFunc, err := createMsg(cmdName, args)
	if msg == nil {
		usage = usageFunc(progName)
		return
	}
	cfg := pmc.NewClientConfig()
	cfg.SetDefaults()
	response, err := sendRecv(cfg, msg)
	if err != nil {
		return
	}
	if response != nil {
		fmt.Printf("Response: %+v\n", response.Value())
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

const summary = `[-h|--help] [--utfOffset N] [--clockClass N] [--clockAccuracy N] [--offsetScaledLogVariance N] [--timeSource N] [--leap61] [--leap59] [--currentUtcOffsetValid] [--ptpTimescale] [--timeTraceable] [--frequencyTraceable] [--get mgmtID]`

func createMsg(cmdName string, args[]string) (pmc.MgmtMsg, func(string) string, error) {
	flags := pflag.NewFlagSet(cmdName, pflag.ContinueOnError)
	help := false
	flags.BoolVarP(&help, "help", "h", false, "show help")

	utcOffset := flags.Int("utcOffset", 37, "TAI-UTC offset in seconds")
	clockClass := hexUint8Flag(flags, "clockClass", 0x6, "clock class")
	clockAccuracy := hexUint8Flag(flags, "clockAccuracy", 0x23, "clock accuracy")
	offsetScaledLogVariance := flags.Uint("offsetScaledLogVariance", 0xFFFF, "offset scaled log variance")
	timeSource := hexUint8Flag(flags, "timeSource", 0x20, "time source")
	timeBoolFlags := map[string]*struct {
		flag  pmc.TimeFlags
		usage string
		set   bool
	}{
		"leap61":                {usage: "positive leap second at end of current UTC day", flag: pmc.Leap61},
		"leap59":                {usage: "negative leap second at end of current UTC day", flag: pmc.Leap59},
		"currentUtcOffsetValid": {usage: "current UTC offset is traceable", flag: pmc.CurrentUTCOffsetValid},
		"ptpTimescale":          {usage: "use PTP timescale", flag: pmc.PTPTimescale},
		"timeTraceable":         {usage: "time is traceable", flag: pmc.TimeTraceable},
		"frequencyTraceable":    {usage: "frequency is traceable", flag: pmc.FrequencyTraceable},
	}
	for name, f := range timeBoolFlags {
		flags.BoolVar(&f.set, name, false, f.usage)
	}
	getMID := flags.Uint("get", 0, "management message ID to get")
	usage := tool.UsageFunc(cmdName, summary, flags)
	err := flags.Parse(args)
	if err != nil {
		return nil, usage, err
	}
	if help {
		return nil, usage, nil
	}
	if *getMID != 0 {
		return pmc.NewMgmtGetMsg(pmc.MgmtID(*getMID)), nil, nil
	}
	var gs pmc.GrandmasterSettings
	gs.UTCOffset = int16(*utcOffset)
	gs.ClockQuality.ClockClass = uint8(*clockClass)
	gs.ClockQuality.ClockAccuracy = uint8(*clockAccuracy)
	gs.ClockQuality.OffsetScaledLogVariance = uint16(*offsetScaledLogVariance)
	gs.TimeSource = pmc.TimeSource(uint8(*timeSource))
	for _, f := range timeBoolFlags {
		if f.set {
			gs.TimeFlags |= f.flag
		}
	}
	return pmc.NewMgmtSetMsg(gs), nil, nil
}