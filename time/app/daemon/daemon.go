package daemon

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/jclark/satpulse/gps/app/bcast"
	"github.com/jclark/satpulse/gps/app/cmd"
	"github.com/jclark/satpulse/gps/app/gpscfg"
	"github.com/jclark/satpulse/gps/app/gpsio"
	"github.com/jclark/satpulse/gps/app/stream"
	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/gpsreg"
	"github.com/jclark/satpulse/gps/ptime"
	"github.com/jclark/satpulse/gps/scan"
	"github.com/jclark/satpulse/time/internal/gpsevent"
	"github.com/jclark/satpulse/time/internal/logobs"
	"github.com/jclark/satpulse/time/internal/obs"
	"github.com/jclark/satpulse/time/internal/phcsync"
	"github.com/jclark/satpulse/time/internal/promobs"
	"github.com/jclark/satpulse/time/internal/proxy"
	"github.com/jclark/satpulse/time/internal/ptpgm"
	"github.com/jclark/satpulse/time/internal/refclock"
	"github.com/jclark/satpulse/time/internal/serialpps"
	"github.com/jclark/satpulse/time/internal/sseobs"
	"github.com/jclark/satpulse/time/internal/ts"
	"github.com/jclark/satpulse/time/lib/ntpshm"
	"github.com/jclark/satpulse/time/lib/pmc"
	"github.com/jclark/satpulse/time/lib/sse"
	"github.com/jclark/satpulse/time/phc"
)

func Cmd(progName string, args []string) {
	vars, msg, err := parseFlags(progName, args)
	if vars == nil {
		exitCode := 0
		if err != nil {
			cmd.ErrPrintln(progName, err)
			exitCode = exitUsage
		}
		fmt.Fprint(os.Stderr, msg)
		os.Exit(exitCode)
	}
	cfg, configPath, err := LoadConfig(vars.configFiles...)
	if err != nil {
		cmd.ErrPrintlnWithDetail(progName, err)
		os.Exit(exitConfig)
	}
	vendors, err := cmd.ResolveVendors(cfg.GPS.Vendor)
	if err != nil {
		cmd.ErrPrintlnWithDetail(progName, err)
		os.Exit(exitConfig)
	}
	if vars.wait {
		cfg.PHC.Wait = true
	}
	if vars.serialDevice != "" {
		cfg.Serial.Device = vars.serialDevice
	}
	level := slog.LevelInfo
	if vars.verbose || cfg.Log.Verbose {
		level = slog.LevelDebug
	}
	var handler slog.Handler
	if vars.sdLog {
		handler = NewSdHandler(level, os.Stdout)
	} else {
		handler = slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: level})
	}
	lg := slog.New(handler)
	slog.SetDefault(lg)
	ver, buildDate := cmd.Version()
	lg.Info("starting", "version", ver, "buildDate", buildDate, "configPath", configPath)
	ctx := context.Background()
	ctx, _ = cmd.CancelOnSignal(ctx, lg)
	ctx, cancelCause := context.WithCancelCause(ctx)
	err = run(ctx, lg, cancelCause, cfg, vendors)
	// run returns only after shutdown completes, so the cancellation cause is
	// settled. A non-signal cause -- the scan worker sets one when the serial
	// input disappears -- becomes the process error, so exitCode picks a
	// non-zero status and systemd restarts us.
	if err == nil {
		if cause := context.Cause(ctx); cause != nil && cause != context.Canceled {
			err = cause
		}
	}
	if err != nil {
		cmd.ErrPrintln(progName, err)
		os.Exit(exitCode(err))
	}
}

func run(ctx context.Context, lg *slog.Logger, cancel context.CancelCauseFunc, cfg *Config, vendors []gpsreg.Vendor) error {
	tStart := time.Now()
	if err := cfg.Validate(lg); err != nil {
		return err
	}
	clk, err := cfg.PHC.OpenClock(ctx, lg)
	if err != nil {
		// don't report an error if interrupted
		if ctx.Err() != nil {
			return nil
		}
		if !errors.Is(err, phc.ErrNotSupported) {
			return err
		}
	}
	if clk == nil {
		lg.Info("no interface specified, running without a PTP hardware clock")
	}
	var phcFlags phc.DriverFlags
	if clk != nil {
		phcFlags = clk.DriverFlags
		lg.Info("selected PTP hardware clock", "path", clk.Path(),
			"known", phcFlags&phc.DriverKnown != 0,
			"bothEdges", phcFlags&phc.DriverBothEdges != 0,
			"oneEdge", phcFlags&phc.DriverOneEdge != 0,
			"poll4Hz", phcFlags&phc.DriverPoll4Hz != 0)
		defer func() {
			clk.Close()
			lg.Debug("closed the PHC", "interface", cfg.PHC.Interface)
		}()
	}
	cfgSpeed := 0
	if cfg.Serial.Speed != nil {
		cfgSpeed = *cfg.Serial.Speed
	}
	conn, speed, err := gpsio.OpenSerial(cfg.Serial.Device, cfgSpeed)
	if err != nil {
		return err
	}
	if cfg.Serial.PPS != nil {
		if _, err := conn.ModemControlPinState(); err != nil {
			conn.Close()
			return fmt.Errorf("serial PPS requires a TTY with modem-control pins: %w", err)
		}
	}
	if speed == 0 {
		speed = cfgSpeed
	}

	// During normal shutdown, the worker-wait defer registered below runs
	// first, so the serial PPS poller stops before the connection is closed.
	defer func() {
		serialDev := cfg.Serial.Device
		lg.Debug("closing the serial port", "path", serialDev)
		e := conn.Close()
		if e != nil {
			lg.Error("error closing the serial port", "path", serialDev, "error", e)
		} else {
			lg.Debug("successfully closed the serial port", "path", serialDev)
		}
	}()

	var wg sync.WaitGroup
	pktFormats := gpsreg.CreatePacketFormats(vendors)
	// pLog must be closed by both the startScan goroutine and the conn
	// gpsio.Scan starts a goroutine that calls conn.Stop() when the context is cancelled
	pLog, lf, err := gpsio.LogPackets(lg, &wg, cfg.Log.PacketPath(cfg.Serial.Device, gpsio.PacketLogExtension), true, pktFormats)
	if err != nil {
		return err
	}
	if lf != nil {
		defer lf.Close(lg)
	}
	if pLog != nil {
		conn.SetPacketLog(pLog)
	}
	pCh := startScan(ctx, lg, &wg, cancel, conn, pLog, pktFormats)

	pb := startBcast(ctx, lg, &wg, pCh)

	var sseCh chan sse.Event
	var eb *bcast.Bcast[sse.Event]
	if cfg.anyHTTP(HTTPConfig.gui) {
		sseCh = make(chan sse.Event, 1)
		eb = startBcast(ctx, lg, &wg, sseCh)
	}
	// Shut down the broadcast goroutines when the context is cancelled.
	wg.Go(func() {
		<-ctx.Done()
		pb.Close()
		if eb != nil {
			eb.Close()
		}
	})
	pCh = pb.Subscribe()
	defer func() {
		// Avoid calling wg.Wait() if we panic
		if r := recover(); r != nil {
			panic(r)
		}
		// startScan starts a goroutine sending to pCh and reading from conn;
		// cancelling here makes those reads fail so pCh is closed and the
		// workers unwind. The cause is irrelevant on this path: run returns err.
		if err != nil {
			cancel(nil)
		}
		// ensure that the sseCh gets closed
		// even if we don't reach the point where the syncWorker does this
		if sseCh != nil {
			close(sseCh)
		}
		// This defer intentionally runs before the serial-port close defer,
		// so that no goroutine still issues syscalls against the connection
		// when it is closed. One exception is safe by construction: a
		// serial-PPS wait abandoned by cancellation may stay parked in
		// TIOCMIWAIT past the close, but it holds a private dup of the
		// descriptor and touches nothing else (see the term package's
		// CancelModemControlPinWait).
		wg.Wait()
		lg.Debug("wait group counter dropped to zero")
	}()

	pktProcs := gpsreg.CreatePacketProcessors(vendors)
	// Install a MsgHandler to capture leap second and position during
	// configuration; consumed after gpscfg.Configure returns.
	var cc configCapture
	gpsprot.SetAllMsgHandlers(pktProcs, &cc)
	// Compile-time check: serial faults surfaced by gpsio satisfy the
	// gpscfg.SerialError interface. gpsInit relies on this.
	var _ gpscfg.SerialError = (*gpsio.SerialError)(nil)
	gct, err := createConfigTarget(lg, cfg, speed, clk != nil)
	if err != nil {
		return err
	}
	gcfg, err := gpscfg.Configure(ctx, lg, pktProcs, gpsreg.CreateConfigProtocols(vendors), gct, pCh, conn)
	cc.logLeapSecond(lg)
	if err != nil {
		if errors.Is(err, gpscfg.ErrNoProbeResponse) {
			lg.Info(err.Error())
		} else if clk == nil && errors.Is(err, gpscfg.ErrNotDetected) {
			lg.Warn(err.Error())
		} else {
			return err
		}
	}
	if ctx.Err() != nil {
		return nil
	}

	portLock := gpsio.NewOutPortLock(conn)
	err = proxy.Start(ctx, lg, &wg, cfg.Proxy, pb, portLock)
	if err != nil {
		return err
	}

	version, _ := cmd.Version()
	pull, pullAddr := cfg.Stream.Pull.NewPull(version, lg,
		gpsreg.CreateCorrectionFormats(), conn, portLock)
	var ggaSelector *stream.GGASelector
	var selectedGGA <-chan scan.Packet
	if pull != nil && cfg.Stream.Pull.NMEASend() {
		ggaSelector = stream.NewGGASelector()
		selectedGGA = ggaSelector.Packets()
	}
	var pullPktCh <-chan scan.Packet
	if pull != nil {
		pullPktCh = pull.Packets.Subscribe()
	}

	if err := startNtrip(ctx, lg, &wg, cfg, gcfg, pb, cc.pos); err != nil {
		return err
	}
	startPush(ctx, lg, &wg, cfg, gcfg, pb, cc.pos, packetTagSet(pktFormats))

	promObs := newPrometheusObserver(cfg)
	sseObs := newSSEObserver(cfg, sseCh, lg, gcfg)
	posObs := newPositionObserver(cfg)
	if len(cfg.HTTP) > 0 {
		err = startHTTP(ctx, lg, &wg, cfg.HTTP, eb, sseObs, promObs, posObs)
		if err != nil {
			return err
		}
	}

	var (
		gm         *ptpgm.Grandmaster
		gmUpdateCh <-chan ptpgm.GrandmasterUpdateRequest
		pmcClient  *pmc.Client
	)
	// The PHC sync controller owns gm.Close(), which in turn shuts down the
	// PTP4L worker by closing its request channel.
	if clk != nil && cfg.PTP.PTP4L != nil {
		pmcClient, err = cfg.PTP.NewClient()
		if err != nil {
			return err
		}
		cq, err := cfg.PTP.ClockQuality()
		if err != nil {
			return err
		}
		gm, gmUpdateCh = ptpgm.NewGrandmaster(ptpgm.Config{
			InSyncClockQuality: cq,
		})
	}

	rc, err := cfg.NTP.NewRefClock(lg)
	if err != nil {
		return err
	}
	shm, err := cfg.NTP.NewSHMWriter(lg)
	if err != nil {
		return err
	}
	var (
		rcProxy *refclock.ProxyRefClock
		rcCh    <-chan refclock.RefClockSample
	)
	if rc != nil {
		rcProxy, rcCh = refclock.NewProxyRefClock()
	}
	var tsCh <-chan ts.Event
	if clk != nil {
		tsCh, err = ts.StartWorker(ctx, clk, lg)
		if err != nil {
			return err
		}
	}
	var serialPPSCh <-chan serialpps.Edge
	var serialPPSGen *serialpps.Generator
	if cfg.Serial.PPS != nil {
		pin, _ := cfg.Serial.PPS.modemControlPin() // checked by Config.Validate
		serialPPSGen = serialpps.NewGenerator(cfg.Sample.Serial.PPS)
		ch := make(chan serialpps.Edge, 1)
		serialPPSCh = ch
		wg.Go(func() {
			defer close(ch)
			lg.Debug("serial PPS goroutine started", "pin", cfg.Serial.PPS.Pin)
			if err := serialpps.Detect(ctx, lg, conn, serialpps.Wiring{Pin: pin}, ch); err != nil && ctx.Err() == nil {
				lg.Error("serial PPS detection failed", "pin", cfg.Serial.PPS.Pin, "err", err)
				cancel(fmt.Errorf("serial PPS detection failed on %s: %w", cfg.Serial.PPS.Pin, err))
			}
			lg.Debug("serial PPS goroutine exited", "pin", cfg.Serial.PPS.Pin)
		})
	}
	statsObs := newStatsLogObserver(cfg, lg)
	clockObs, err := newClockLogObserver(cfg, lg, clk, cfg.LeapSecond.leapSecond())
	if err != nil {
		return err
	}
	trackObs, err := newTrackLogObserver(cfg, lg)
	if err != nil {
		return err
	}
	gpsObs := logobs.NewGPSLogObserver(lg)
	ubxObs := logobs.NewUBXLogObserver(lg)

	var oc obs.ObserverCombiner
	obs.AddObserver(&oc, statsObs)
	obs.AddObserver(&oc, clockObs)
	obs.AddObserver(&oc, trackObs)
	obs.AddObserver(&oc, gpsObs)
	obs.AddObserver(&oc, ubxObs)
	obs.AddObserver(&oc, promObs)
	obs.AddObserver(&oc, sseObs)
	obs.AddObserver(&oc, posObs)
	observer := oc.Observer()

	d, err := NewDispatcher(lg, pktProcs, clk, cfg, gm, rcProxy, shm, serialPPSGen, observer, tStart, ggaSelector)
	if err != nil {
		return err
	}

	if pmcClient != nil {
		wg.Go(func() { ptpgm.PTP4LWorker(pmcClient, gmUpdateCh, lg) })
	}
	if rc != nil {
		wg.Go(func() { refclock.RefClockWorker(rc, rcCh, lg) })
	}
	// the SyncRunner assumes responsibility for closing the sseCh
	sseCh = nil
	ls := cc.leapSecond
	startPull(ctx, lg, &wg, pull, pullAddr, selectedGGA)
	wg.Go(func() {
		if ls != nil {
			d.LeapSecond(ls, time.Time{})
		}
		// Dispatcher is responsible for closing rcProxy via defer in Run()
		d.Run(tsCh, serialPPSCh, pCh, pullPktCh)
	})

	return nil
}

// NewDispatcher creates the GPS event dispatcher for the daemon.
func NewDispatcher(
	lg *slog.Logger,
	pktProcs map[gpsprot.Tag]gpsprot.PacketProcessor,
	clk *ts.Clock,
	cfg *Config,
	gm *ptpgm.Grandmaster,
	rc *refclock.ProxyRefClock,
	shm *ntpshm.Writer,
	serialPPS *serialpps.Generator,
	obs obs.Observer,
	tStart time.Time,
	ggaSelector *stream.GGASelector,
) (*gpsevent.Dispatcher, error) {
	ls := cfg.LeapSecond.leapSecond()
	var controller *phcsync.Controller
	if clk != nil {
		var err error
		controller, err = phcsync.NewController(
			clk,
			obs,
			gm,
			cfg.Sync,
			ls,
			clk.DriverFlags.Edges(),
			lg,
		)
		if err != nil {
			return nil, err
		}
	}
	eventLogPath := cfg.Log.EventPath(cfg.Serial.Device, gpsevent.LogExtension)
	shmWriter := gpsevent.NewSHMWriter(shm, cfg.shmFixedPrecision())
	// Keep a nil selector a nil interface, not a non-nil interface wrapping a
	// nil pointer, which would defeat the dispatcher's nil checks.
	var gs gpsevent.GGASelector
	if ggaSelector != nil {
		gs = ggaSelector
	}
	return gpsevent.NewDispatcher(lg, pktProcs, controller, rc, shmWriter, serialPPS, ls, obs, eventLogPath, tStart, gs)
}

// newSSEObserver creates SSE observer if any HTTP endpoint needs GUI
func newSSEObserver(cfg *Config, sseCh chan<- sse.Event, lg *slog.Logger, gcfg *gpscfg.Result) *sseobs.SSEObserver {
	if cfg.anyHTTP(HTTPConfig.gui) {
		return sseobs.New(sseCh, cfg.LeapSecond.leapSecond(), lg, gcfg)
	}
	return nil
}

// newPrometheusObserver creates Prometheus observer if any HTTP endpoint needs metrics
func newPrometheusObserver(cfg *Config) *promobs.PrometheusObserver {
	if cfg.anyHTTP(HTTPConfig.metrics) {
		return promobs.New(cfg.PTP.ClockAccuracy)
	}
	return nil
}

// newPositionObserver creates a positionObserver if any HTTP endpoint has position enabled.
func newPositionObserver(cfg *Config) *positionObserver {
	if cfg.anyHTTP(HTTPConfig.position) {
		return &positionObserver{}
	}
	return nil
}

// newStatsLogObserver creates StatsLogObserver if log interval is configured
func newStatsLogObserver(cfg *Config, lg *slog.Logger) *logobs.StatsLogObserver {
	if cfg.Log.Interval > 0 {
		return logobs.NewStatsLogObserver(lg, cfg.Log.Interval)
	}
	return nil
}

// newTrackLogObserver creates TrackLogObserver if track log path is configured
func newTrackLogObserver(cfg *Config, lg *slog.Logger) (*logobs.TrackLogObserver, error) {
	path := cfg.Log.TrackPath(cfg.Serial.Device, logobs.TrackLogExtension)
	if path == "" {
		return nil, nil
	}
	return logobs.NewTrackLogObserver(lg, path)
}

// newClockLogObserver creates ClockLogObserver if clock log path is configured
func newClockLogObserver(cfg *Config, lg *slog.Logger, clk *ts.Clock, ls ptime.LeapSecond) (*logobs.ClockLogObserver, error) {
	if clk == nil {
		return nil, nil
	}
	clockLogPath := cfg.Log.ClockPath(clk.IfName(), logobs.ClockLogExtension)
	if clockLogPath == "" {
		return nil, nil
	}
	return logobs.NewClockLogObserver(lg, clockLogPath, ls)
}

func startScan(ctx context.Context, lg *slog.Logger, wg *sync.WaitGroup, cancel context.CancelCauseFunc, conn gpsio.Conn, pLog *gpsio.PacketLog, pktFormats []gpsprot.PacketFormat) <-chan scan.Packet {
	msg := make(chan scan.Packet, 1)
	wg.Go(func() {
		gpsio.Scan(ctx, lg, conn, msg, pLog, pktFormats)
		// Scan returns only when the GPS data source is gone. If we weren't
		// already shutting down, the serial input reached EOF (the device
		// disappeared): cancel with a cause so the daemon shuts down and Cmd
		// exits non-zero to be restarted.
		if ctx.Err() == nil {
			cancel(fmt.Errorf("serial device no longer providing input: %s", conn.LocalAddr()))
		}
	})
	return msg
}

func startBcast[T any](ctx context.Context, lg *slog.Logger, wg *sync.WaitGroup, msg <-chan T) *bcast.Bcast[T] {
	b := bcast.New(msg)
	wg.Go(func() { b.Run(ctx, lg) })
	return b
}

// configCapture is a MsgHandler that opportunistically captures
// leap second and geodetic position messages received during GPS
// configuration, for use by code that runs after gpscfg.Configure.
type configCapture struct {
	gpsprot.DefaultHandler
	leapSecond *gpsprot.LeapSecondMsg
	pos        *gpsprot.PosGeoMsg
}

func (h *configCapture) LeapSecond(msg *gpsprot.LeapSecondMsg, _ time.Time) {
	h.leapSecond = msg
}

func (h *configCapture) PosGeo(msg *gpsprot.PosGeoMsg, _ time.Time) {
	h.pos = msg
}

func (h *configCapture) logLeapSecond(lg *slog.Logger) {
	if h.leapSecond != nil {
		lsdStr := h.leapSecond.Date().Format("2006-01-02")
		lg.Info("leap second information received from GPS", "date", lsdStr, "utcOffBefore", h.leapSecond.UTCOffBefore, "utcOffAfter", h.leapSecond.UTCOffAfter)
	}
}

func createConfigTarget(lg *slog.Logger, cfg *Config, speed int, usingPHC bool) (*gpsprot.ConfigTarget, error) {
	gct, err := cfg.GPS.target(speed, configFeatures(cfg, usingPHC))
	lg.Debug("GPS configure input", "target", gct)
	if err != nil {
		if errors.Is(err, errSatsOutNotEnabled) {
			lg.Warn(err.Error())
		} else {
			return nil, err
		}
	}
	return gct, nil
}

func configFeatures(cfg *Config, usingPHC bool) cfgFeatures {
	var cf cfgFeatures
	if usingPHC {
		cf |= cfgTimePulse | cfgTimePulseMsg
	} else if (cfg.NTP.Sock != nil && cfg.NTP.Sock.Path != "") || (cfg.NTP.SHM != nil && cfg.NTP.SHM.Segment != nil) {
		cf |= cfgTimePulse
	}
	if cfg.Log.Track || len(cfg.HTTP) > 0 {
		cf |= cfgPosition
	}
	if cfg.Stream.Pull.NMEASend() {
		cf |= cfgPosition
	}
	if cfg.httpWantsSatellites() {
		cf |= cfgSatellites
	}
	if cfg.hasNtripStream() {
		cf |= cfgNtripStream
	}
	if cfg.hasRTCMMSM7To4() {
		cf |= cfgRTCMMSM7To4
	}
	return cf
}
