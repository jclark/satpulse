package gpscmd

import (
	"encoding/json"
	"log/slog"
	"os"

	"github.com/jclark/satpulse/internal/cmd"
	"github.com/jclark/satpulse/internal/gpscfg"
	"github.com/jclark/satpulse/internal/gpsprot"
	"github.com/jclark/satpulse/internal/logfile"
	"github.com/jclark/satpulse/internal/ubx"
)

type TestLogEnvEntry struct {
	Type     string   `json:"type"`
	Args     []string `json:"args"`
	Version  string   `json:"version"`
	Hostname string   `json:"hostname"`
}

type TestLogReceiverEntry struct {
	Type     string `json:"type"`
	Model    string `json:"model,omitempty"`
	Firmware string `json:"firmware,omitempty"`
	Protocol string `json:"protocol,omitempty"`
}

type TestLogConfigEntry struct {
	Type           string     `json:"type"`
	Error          string     `json:"error,omitempty"`
	SignalsEnabled [][]string `json:"signalsEnabled,omitempty"`
	BaudRate       *uint32    `json:"baudRate,omitempty"`
	Static         *bool      `json:"static,omitempty"`
	FixedPosECEF   []float64  `json:"fixedPosECEF,omitempty"`
	FixedPosAcc    *float64   `json:"fixedPosAcc,omitempty"`
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
		writeTestLogReceiver(lf, lg, rslt.Version)
		props = rslt.ConfigProps
	}
	writeTestLogConfigProps(lf, lg, props, err)
}

func writeTestLogReceiver(lf *logfile.LogFile, lg *slog.Logger, version *ubx.Version) {
	if version == nil {
		return
	}
	entry := TestLogReceiverEntry{
		Type:  "receiver",
		Model: version.Mod,
	}
	if version.FW != nil {
		entry.Firmware = version.FW.String()
	}
	if version.Prot != nil {
		entry.Protocol = version.Prot.String()
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
