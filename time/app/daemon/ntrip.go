package daemon

import (
	"context"
	"log/slog"
	"sync"

	"github.com/jclark/satpulse/gps/app/bcast"
	"github.com/jclark/satpulse/gps/app/cmd"
	"github.com/jclark/satpulse/gps/app/gpscfg"
	"github.com/jclark/satpulse/gps/app/ntrip"
	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/scan"
)

// startNtrip wires the Ntrip caster into the daemon.  When [ntrip]
// is configured (mountpoints present), it builds the shared
// StreamRecordBuilder closure and calls ntrip.Start.  When no
// mountpoints are configured, returns nil and does nothing.
func startNtrip(ctx context.Context, lg *slog.Logger, wg *sync.WaitGroup,
	cfg *Config, gcfg *gpscfg.Result, pb *bcast.Bcast[scan.Packet]) error {
	if len(cfg.Ntrip.Mountpoint) == 0 {
		return nil
	}
	msm := 0
	if cfg.GPS.Config && cfg.GPS.RTCMOutput != nil && *cfg.GPS.RTCMOutput {
		msm = 4
	}
	hasAuth := len(cfg.Ntrip.Users) > 0
	var props *gpsprot.ConfigProps
	var info *gpsprot.ReceiverInfo
	if gcfg != nil {
		props = gcfg.ConfigProps
		info = gcfg.ReceiverInfo
	}
	buildSTR := ntrip.StreamRecordBuilder(
		&cfg.Ntrip.SharedStreamConfig, props, info,
		cmd.VersionInfo(), msm, hasAuth)
	version, _ := cmd.Version()
	return ntrip.Start(ctx, lg, wg, cfg.Ntrip, version, pb, buildSTR)
}
