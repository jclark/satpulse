package daemon

import (
	"context"
	"errors"
	"log/slog"
	"sync"

	"github.com/jclark/satpulse/gps/app/bcast"
	"github.com/jclark/satpulse/gps/app/cmd"
	"github.com/jclark/satpulse/gps/app/gpscfg"
	"github.com/jclark/satpulse/gps/app/ntrip"
	"github.com/jclark/satpulse/gps/app/stream"
	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/gpsreg"
	"github.com/jclark/satpulse/gps/scan"
)

// startPull spawns the stream.pull goroutine.  It is a no-op when
// setup is nil.  A non-cancel error from Setup.Run is logged but
// does not cancel the daemon: time/PHC sync is independent of
// corrections, and the daemon should continue degraded rather than
// tear down on a correction-side fault.
func startPull(ctx context.Context, lg *slog.Logger,
	wg *sync.WaitGroup, setup *stream.PullSetup) {
	if setup == nil {
		return
	}
	addr := setup.Addr()
	onState := func(st stream.State, err error) {
		switch st {
		case stream.Connecting:
			lg.Info("stream pull connecting", "addr", addr)
		case stream.Connected:
			lg.Info("stream pull connected", "addr", addr)
		case stream.Reconnecting:
			lg.Warn("stream pull reconnecting", "addr", addr, "err", err)
		}
	}
	wg.Go(func() {
		err := setup.Run(ctx, lg, onState)
		if err != nil && !errors.Is(err, context.Canceled) {
			lg.Error("stream pull exited with error", "addr", addr, "err", err)
		}
	})
}

// packetTagSet returns the set of tags supported by the given
// packet formats, for cheap tag-membership tests.
func packetTagSet(formats []gpsprot.PacketFormat) map[gpsprot.Tag]struct{} {
	tags := make(map[gpsprot.Tag]struct{}, len(formats))
	for _, f := range formats {
		tags[f.Tag()] = struct{}{}
	}
	return tags
}

// startPush wires stream.push entries into the daemon.  It is
// independent of startNtrip -- the two share GPS state inputs but
// build their own StreamRecordBuilder closures.  validTags is the
// set of tags supported by the receiver's configured packet formats;
// an entry whose non-empty protocol tag is absent is skipped with a
// warning.
func startPush(ctx context.Context, lg *slog.Logger, wg *sync.WaitGroup,
	cfg *Config, gcfg *gpscfg.Result, pb *bcast.Bcast[scan.Packet],
	configCapturePos *gpsprot.PosGeoMsg,
	validTags map[gpsprot.Tag]struct{}) {
	if len(cfg.Stream.Push) == 0 {
		return
	}
	var buildSTR func(sc *ntrip.StreamConfig, name string, hasAuth, msm7to4 bool) string
	if hasRTCMPush(cfg) {
		var props *gpsprot.ConfigProps
		var info *gpsprot.ReceiverInfo
		if gcfg != nil {
			props = gcfg.ConfigProps
			info = gcfg.ReceiverInfo
		}
		buildSTR = ntrip.StreamRecordBuilder(
			&cfg.Ntrip.SharedStreamConfig, props, info,
			cmd.VersionInfo(), rtcmOutputSupport(cfg, gcfg), configCapturePos)
	}
	version, _ := cmd.Version()
	for i := range cfg.Stream.Push {
		p := &cfg.Stream.Push[i]
		tag := p.Protocol.Tag()
		if !tag.IsZero() {
			if _, ok := validTags[tag]; !ok {
				switch {
				case p.Ntrip != nil:
					lg.Warn("ntrip push skipped: protocol not supported by this vendor",
						"mountpoint", p.Ntrip.Mountpoint, "protocol", tag)
				case p.UDP != nil:
					lg.Warn("udp push skipped: protocol not supported by this vendor",
						"addr", p.UDP.Address, "protocol", tag)
				}
				continue
			}
		}
		switch {
		case p.Ntrip != nil:
			dest := &stream.NtripDestination{
				Addr:       stream.NtripNormalizeAddress(p.Ntrip.Address),
				Mountpoint: p.Ntrip.Mountpoint,
				Password:   p.Ntrip.Password,
				UserAgent:  stream.NtripUserAgent{Version: version},
			}
			if tag == gpsreg.TagRTCM && buildSTR != nil {
				dest.STR = buildSTR(&p.Ntrip.StreamConfig, p.Ntrip.Mountpoint, false, p.Ntrip.MSM7to4)
			}
			runPushEntry(ctx, lg, wg, pb, dest, tag, p.Ntrip.MSM7to4)
		case p.UDP != nil:
			runUDPPushEntry(ctx, lg, wg, pb, p.UDP.Address, tag)
		}
	}
}

// runPushEntry starts one Push.Run goroutine with state-change
// logging tagged by destination address and mountpoint.  Kept
// separate from the configuration loop so the captured variables
// are scoped to a single entry.
func runPushEntry(ctx context.Context, lg *slog.Logger, wg *sync.WaitGroup,
	pb *bcast.Bcast[scan.Packet], dest *stream.NtripDestination,
	pktTag gpsprot.Tag, msm7to4 bool) {
	addr := dest.Addr
	mnt := dest.Mountpoint
	onState := func(st stream.State, err error) {
		switch st {
		case stream.Connecting:
			lg.Info("ntrip push connecting", "addr", addr, "mountpoint", mnt)
		case stream.Connected:
			lg.Info("ntrip push connected", "addr", addr, "mountpoint", mnt)
		case stream.Reconnecting:
			lg.Warn("ntrip push reconnecting", "addr", addr, "mountpoint", mnt, "err", err)
		case stream.Failed:
			lg.Error("ntrip push gave up", "addr", addr, "mountpoint", mnt, "err", err)
		}
	}
	push := stream.NewPush()
	wg.Go(func() {
		err := push.Run(ctx, lg, pb, dest, pktTag, msm7to4, onState)
		if err != nil && !errors.Is(err, context.Canceled) {
			lg.Error("ntrip push exited with error", "addr", addr, "mountpoint", mnt, "err", err)
		}
	})
}

func runUDPPushEntry(ctx context.Context, lg *slog.Logger, wg *sync.WaitGroup,
	pb *bcast.Bcast[scan.Packet], addr string, pktTag gpsprot.Tag) {
	sink := &stream.UDPSink{Addr: addr}
	wg.Go(func() {
		lg.Info("udp push starting", "addr", addr, "protocol", pktTag)
		err := sink.Run(ctx, lg, pb, pktTag)
		if err != nil && !errors.Is(err, context.Canceled) {
			lg.Error("udp push exited with error", "addr", addr, "protocol", pktTag, "err", err)
		}
	})
}

// hasRTCMPush reports whether any Ntrip push entry forwards RTCM.
// Used to decide whether to build the StreamRecordBuilder closure.
func hasRTCMPush(cfg *Config) bool {
	for i := range cfg.Stream.Push {
		p := &cfg.Stream.Push[i]
		if p.Ntrip != nil && p.Protocol.Tag() == gpsreg.TagRTCM {
			return true
		}
	}
	return false
}

func hasRTCMPushMSM7To4(cfg *Config) bool {
	for i := range cfg.Stream.Push {
		p := &cfg.Stream.Push[i]
		if p.Ntrip != nil && p.Protocol.Tag() == gpsreg.TagRTCM && p.Ntrip.MSM7to4 {
			return true
		}
	}
	return false
}
