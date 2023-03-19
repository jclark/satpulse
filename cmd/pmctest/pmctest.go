package main

import (
	"flag"
	"fmt"
	"os"
	"strconv"

	"github.com/jclark/gps2phc/internal/pmc"
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

func hexUint8Flag(name string, value hexUint8, usage string) *hexUint8 {
	flag.CommandLine.Var(&value, name, usage)
	return &value
}

const localSocketPathFormat = "/tmp/gps2phc%d.sock"

func main() {
	msg := createMsg()
	cfg := pmc.NewClientConfig()
	cfg.LocalSocketPathFormat = localSocketPathFormat
	response, err := send(cfg, msg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Response: %+v\n", response)
}

func send(cfg *pmc.ClientConfig, msg pmc.MgmtMsg) (pmc.MgmtMsg, error) {
	client, err := pmc.NewClient(cfg)
	if err != nil {
		return nil, err
	}
	return client.Send(msg)
}

func createMsg() pmc.MgmtMsg {
	utcOffset := flag.Int("utcOffset", 37, "TAI-UTC offset in seconds")
	clockClass := hexUint8Flag("clockClass", 0x6, "clock class")
	clockAccuracy := hexUint8Flag("clockAccuracy", 0x23, "clock accuracy")
	offsetScaledLogVariance := flag.Uint("offsetScaledLogVariance", 0xFFFF, "offset scaled log variance")
	timeSource := hexUint8Flag("timeSource", 0x20, "time source")
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
		flag.BoolVar(&f.set, name, false, f.usage)
	}
	getMID := flag.Uint("get", 0, "management message ID to get")
	flag.Parse()
	if *getMID != 0 {
		return pmc.NewMgmtGetMsg(pmc.MgmtID(*getMID))
	}
	var gs pmc.GrandmasterSettings
	gs.UTCOffset = int16(*utcOffset)
	gs.ClockQuality.ClockClass = uint8(*clockClass)
	gs.ClockQuality.ClockAccuracy = uint8(*clockAccuracy)
	gs.ClockQuality.OffsetScaledLogVariance = uint16(*offsetScaledLogVariance)
	gs.TimeSource = uint8(*timeSource)
	for _, f := range timeBoolFlags {
		if f.set {
			gs.TimeFlags |= f.flag
		}
	}
	return pmc.NewMgmtSetMsg(gs)
}
