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
	Type           string      `json:"type"`
	SignalsEnabled [][]string `json:"signalsEnabled,omitempty"`
	BaudRate       *uint32     `json:"baudRate,omitempty"`
	NMEAEnabled    *bool       `json:"nmeaEnabled,omitempty"`
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

func writeTestLogTail(lf *logfile.LogFile, lg *slog.Logger, rslt *gpscfg.Result) {
	writeTestLogConfigProps(lf, lg, rslt.ConfigProps)
	writeTestLogReceiver(lf, lg, rslt.Version)
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

func writeTestLogConfigProps(lf *logfile.LogFile, lg *slog.Logger, props *gpsprot.ConfigProps) {
	if props == nil {
		return
	}
	entry := TestLogConfigEntry{
		Type: "config",
	}
	if sigs, ok := props.GetSignalsEnabled(); ok {
		entry.SignalsEnabled = sigs.GNSSStringGroups()
	}
	if br, ok := props.GetBaudRate(); ok {
		entry.BaudRate = &br
	}
	if nmea, ok := props.GetNMEAEnabled(); ok {
		entry.NMEAEnabled = &nmea
	}
	writeTestLogEntry(lf, lg, entry)
}

func writeTestLogEntry(lf *logfile.LogFile, lg *slog.Logger, entry interface{}) {
	bytes, err := json.Marshal(entry)
	if err != nil {
		lg.Warn("failed to convert test log entry to JSON", "err", err)
		return
	}
	bytes = append(bytes, '\n')
	_, err = lf.File.Write(bytes)
	lf.HandleWriteError(err, lg)
}
