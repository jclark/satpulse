package gpscmd

import (
	"encoding/json"
	"log/slog"
	"os"

	"github.com/jclark/satpulse/internal/cmd"
	"github.com/jclark/satpulse/internal/gpscfg"
	"github.com/jclark/satpulse/internal/gpsprot"
	"github.com/jclark/satpulse/internal/logfile"
)

type TestLogEnvEntry struct {
	Type     string   `json:"type"`
	Args     []string `json:"args"`
	Version  string   `json:"version"`
	Hostname string   `json:"hostname"`
}

type TestLogReceiverEntry struct {
	Type     string         `json:"type"`
	Vendor   string         `json:"vendor,omitempty"`
	Hardware string         `json:"hardware,omitempty"`
	Firmware string         `json:"firmware,omitempty"`
	GNSS     []gpsprot.GNSS `json:"gnss,omitempty"` // GNSS systems supported by the receiver
}

type TestLogConfigEntry struct {
	Type              string           `json:"type"`
	Error             string           `json:"error,omitempty"`
	SignalsEnabled    [][]string       `json:"signalsEnabled,omitempty"`
	TimeGNSS          string           `json:"timeGNSS,omitempty"`
	AntennaCableDelay *int64           `json:"antennaCableDelay,omitempty"`
	TimePulse         *TimePulseConfig `json:"timePulse,omitempty"`
	BaudRate          *uint32          `json:"baudRate,omitempty"`
	Static            *bool            `json:"static,omitempty"`
	FixedPosECEF      []float64        `json:"fixedPosECEF,omitempty"`
	FixedPosAcc       *float64         `json:"fixedPosAcc,omitempty"`
}

type TimePulseConfig struct {
	Enabled        bool     `json:"enabled"`
	Width          *float64 `json:"width,omitempty"`          // in seconds
	Period         *float64 `json:"period,omitempty"`         // in seconds
	PolarityRising *bool    `json:"polarityRising,omitempty"` // true for rising edge, false for falling edge
	OnlyWhenLocked *bool    `json:"onlyWhenLocked,omitempty"` // true if pulse only when receiver is locked
}

func (tp *TimePulseConfig) timePulseEnabled() bool {
	return tp.Enabled
}

func writeTestLogHead(lf *logfile.LogFile, lg *slog.Logger, args []string) {
	hostname, _ := os.Hostname()
	writeTestLogEntry(lf, lg, TestLogEnvEntry{
		Type:     "env",
		Args:     args,
		Version:  cmd.VersionInfo(),
		Hostname: hostname,
	})
}

func writeTestLogTail(lf *logfile.LogFile, lg *slog.Logger, rslt *gpscfg.Result, err error) {
	var props *gpsprot.ConfigProps
	if rslt != nil {
		writeTestLogReceiver(lf, lg, rslt.ReceiverInfo)
		props = rslt.ConfigProps
	}
	writeTestLogConfigProps(lf, lg, props, err)
}

func writeTestLogReceiver(lf *logfile.LogFile, lg *slog.Logger, rcvrInfo *gpsprot.ReceiverInfo) {
	if rcvrInfo == nil {
		return
	}
	entry := TestLogReceiverEntry{
		Type:     "receiver",
		Hardware: rcvrInfo.Hardware,
		Firmware: rcvrInfo.Firmware,
		Vendor:   rcvrInfo.Vendor,
		GNSS:     rcvrInfo.SupportedGNSS.Items(),
	}
	writeTestLogEntry(lf, lg, entry)
}

func writeTestLogConfigProps(lf *logfile.LogFile, lg *slog.Logger, props *gpsprot.ConfigProps, err error) {
	entry := TestLogConfigEntry{
		Type: "config",
	}
	if err != nil {
		entry.Error = err.Error()
	} else if props != nil {
		if sigs, ok := props.GetSignalsEnabled(); ok {
			entry.SignalsEnabled = sigs.GNSSStringGroups()
		}
		if timeGNSS, ok := props.GetTimeGNSS(); ok {
			entry.TimeGNSS = timeGNSS.String()
		}
		if antCableDelay, ok := props.GetAntennaCableDelay(); ok {
			delay := int64(antCableDelay.Nanoseconds())
			entry.AntennaCableDelay = &delay
		}
		if tp, ok := props.GetTimePulse(); ok {
			if tp.Width == 0 {
				entry.TimePulse = &TimePulseConfig{Enabled: false}
			} else {
				width := tp.Width.Seconds()
				period := tp.Period.Seconds()
				entry.TimePulse = &TimePulseConfig{
					Enabled:        true,
					Width:          &width,
					Period:         &period,
					PolarityRising: &tp.PolarityRising,
					OnlyWhenLocked: &tp.OnlyWhenLocked,
				}
			}
		}
		if mode, ok := props.GetMode(); ok {
			static := mode.Static
			entry.Static = &static
			switch mode.PosType {
			case gpsprot.PosTypeECEF:
				for i := range 3 {
					entry.FixedPosECEF = append(entry.FixedPosECEF, float64(mode.FixedPosECEF[i].Meters()))
				}
				acc := mode.FixedPosAcc.Meters()
				entry.FixedPosAcc = &acc
			}
		}
	}
	writeTestLogEntry(lf, lg, entry)
}

func writeTestLogEntry(lf *logfile.LogFile, lg *slog.Logger, entry any) {
	bytes, err := json.Marshal(entry)
	if err != nil {
		lg.Warn("failed to convert test log entry to JSON", "err", err)
		return
	}
	bytes = append(bytes, '\n')
	_, err = lf.File.Write(bytes)
	lf.HandleWriteError(err, lg)
}
