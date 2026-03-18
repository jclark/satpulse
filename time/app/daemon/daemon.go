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
	"github.com/jclark/satpulse/time/internal/gpsevent"
	"github.com/jclark/satpulse/gps/app/gpsio"
	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/time/internal/logobs"
	"github.com/jclark/satpulse/time/internal/obs"
	"github.com/jclark/satpulse/time/phc"
	"github.com/jclark/satpulse/time/internal/phcsync"
	"github.com/jclark/satpulse/time/internal/promobs"
	"github.com/jclark/satpulse/time/internal/proxy"
	"github.com/jclark/satpulse/gps/ptime"
	"github.com/jclark/satpulse/time/internal/ptpgm"
	"github.com/jclark/satpulse/time/internal/refclock"
	"github.com/jclark/satpulse/gps/scan"
	"github.com/jclark/satpulse/time/lib/sse"
	"github.com/jclark/satpulse/time/internal/sseobs"
	"github.com/jclark/satpulse/time/internal/ts"
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
	ctx, cancel := cmd.CancelOnSignal(ctx, lg)
	err = run(ctx, lg, cancel, cfg)
	if err != nil {
		cmd.ErrPrintln(progName, err)
		os.Exit(exitCode(err))
	}
}

func run(ctx context.Context, lg *slog.Logger, cancel context.CancelFunc, cfg *Config) error {
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
	speed := 0
	if cfg.Serial.Speed != nil {
		speed = *cfg.Serial.Speed
	}
	conn, err := gpsio.OpenSerial(cfg.Serial.Device, speed)
	if err != nil {
		return err
	}

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
	// pLog must be closed by both the startScan goroutine and the conn
	// gpsio.Scan starts a goroutine that calls conn.Stop() when the context is cancelled
	pLog, lf, err := gpsio.LogPackets(lg, &wg, cfg.Log.PacketPath(cfg.Serial.Device, gpsio.PacketLogExtension), cfg.GPS.CreatePacketFormats())
	if err != nil {
		return err
	}
	if lf != nil {
		defer lf.Close(lg)
	}
	if pLog != nil {
		conn.SetPacketLog(pLog)
	}
	pCh := startScan(ctx, lg, &wg, conn, pLog, cfg.GPS.CreatePacketFormats())

	pb := startBcast(ctx, lg, &wg, pCh)

	var sseCh chan sse.Event
	var eb *bcast.Bcast[sse.Event]
	if len(cfg.HTTP) > 0 {
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
		// startScan starts a goroutine sending to pCh and reading from conn
		// calling cancel here will cause reads from conn to return with an io.EOF error
		// which will cause pCh to be closed
		if err != nil {
			cancel()
		}
		// ensure that the sseCh gets closed
		// even if we don't reach the point where the syncWorker does this
		if sseCh != nil {
			close(sseCh)
		}
		wg.Wait()
		lg.Debug("wait group counter dropped to zero")
	}()

	pktProcs := cfg.GPS.CreatePacketProcessors()
	// Install a MsgHandler to capture leap second during configuration
	var lsc leapSecondCapture
	gpsprot.SetAllMsgHandlers(pktProcs, &lsc)
	// Let the compiler check that TermError implements the SerialError interface
	// gpsInit relies on this
	var _ gpscfg.SerialError = gpsio.TermError{}
	timePulseEnabled := clk != nil
	gct, err := createConfigTarget(lg, cfg, conn.Speed(), timePulseEnabled)
	if err != nil {
		return err
	}
	gcfg, err := gpscfg.Configure(ctx, lg, pktProcs, cfg.GPS.CreateConfigProtocols(), gct, pCh, conn)
	lsc.logLeapSecond(lg)
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

	promObs := newPrometheusObserver(cfg)
	sseObs := newSSEObserver(cfg, sseCh, lg, gcfg)
	posObs := newPositionObserver(cfg)
	if eb != nil {
		err = startHTTP(ctx, lg, &wg, cfg.HTTP, eb, sseObs, promObs, posObs)
		if err != nil {
			return err
		}
	}

	var (
		gm         *ptpgm.Grandmaster
		gmUpdateCh <-chan ptpgm.GrandmasterUpdateRequest
	)
	pmcClient, err := cfg.PTP.NewClient()
	if err != nil {
		return err
	}
	if pmcClient != nil {
		cq, err := cfg.PTP.ClockQuality()
		if err != nil {
			return err
		}
		gm, gmUpdateCh = ptpgm.NewGrandmaster(ptpgm.Config{
			InSyncClockQuality: cq,
		})
	}

	rc, err := cfg.NTP.NewRefClock(lg)
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

	observer := combineObservers(promObs, sseObs, posObs, statsObs, clockObs, trackObs, gpsObs)

	d, err := NewDispatcher(lg, pktProcs, clk, cfg, gm, rcProxy, observer, tStart)
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
	ls := lsc.msg
	wg.Go(func() {
		if ls != nil {
			d.LeapSecond(ls, time.Time{})
		}
		// Dispatcher is responsible for closing rcProxy via defer in Run()
		d.Run(tsCh, pCh)
	})

	return nil
}

func NewDispatcher(lg *slog.Logger, pktProcs map[gpsprot.Tag]gpsprot.PacketProcessor, clk *ts.Clock, cfg *Config, gm *ptpgm.Grandmaster, rc *refclock.ProxyRefClock, obs obs.Observer, tStart time.Time) (*gpsevent.Dispatcher, error) {
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
	return gpsevent.NewDispatcher(lg, pktProcs, controller, rc, ls, obs, eventLogPath, tStart)
}

// newSSEObserver creates SSE observer if any HTTP endpoint needs GUI
func newSSEObserver(cfg *Config, sseCh chan<- sse.Event, lg *slog.Logger, gcfg *gpscfg.Result) *sseobs.SSEObserver {
	for _, hc := range cfg.HTTP {
		if hc.gui() {
			return sseobs.New(sseCh, cfg.LeapSecond.leapSecond(), lg, gcfg)
		}
	}
	return nil
}

// newPrometheusObserver creates Prometheus observer if any HTTP endpoint needs metrics
func newPrometheusObserver(cfg *Config) *promobs.PrometheusObserver {
	for _, hc := range cfg.HTTP {
		if hc.metrics() {
			return promobs.New(cfg.PTP.ClockAccuracy)
		}
	}
	return nil
}

// newPositionObserver creates a positionObserver if any HTTP endpoint has position enabled.
func newPositionObserver(cfg *Config) *positionObserver {
	for _, hc := range cfg.HTTP {
		if hc.position() {
			return &positionObserver{}
		}
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

// combineObservers combines individual observers into appropriate single observer
func combineObservers(promObs *promobs.PrometheusObserver, sseObs *sseobs.SSEObserver,
	posObs *positionObserver, statsObs *logobs.StatsLogObserver, clockObs *logobs.ClockLogObserver,
	trackObs *logobs.TrackLogObserver, gpsObs *logobs.GPSLogObserver) obs.Observer {

	var observers []obs.Observer

	// Add observers if they exist (typed nils will be properly handled)
	if statsObs != nil {
		observers = append(observers, statsObs)
	}
	if clockObs != nil {
		observers = append(observers, clockObs)
	}
	if trackObs != nil {
		observers = append(observers, trackObs)
	}
	if gpsObs != nil {
		observers = append(observers, gpsObs)
	}
	if promObs != nil {
		observers = append(observers, promObs)
	}
	if sseObs != nil {
		observers = append(observers, sseObs)
	}
	if posObs != nil {
		observers = append(observers, posObs)
	}

	// Return combined observer or default
	if len(observers) == 0 {
		return &obs.DefaultObserver{}
	} else if len(observers) == 1 {
		return observers[0]
	} else {
		return obs.NewMultiObserver(observers...)
	}
}

func startScan(ctx context.Context, lg *slog.Logger, wg *sync.WaitGroup, conn gpsio.Conn, pLog *gpsio.PacketLog, pktFormats []gpsprot.PacketFormat) <-chan scan.Packet {
	msg := make(chan scan.Packet, 1)
	wg.Go(func() { gpsio.Scan(ctx, lg, conn, msg, pLog, pktFormats) })
	return msg
}

func startBcast[T any](ctx context.Context, lg *slog.Logger, wg *sync.WaitGroup, msg <-chan T) *bcast.Bcast[T] {
	b := bcast.New(msg)
	wg.Go(func() { b.Run(ctx, lg) })
	return b
}

// leapSecondCapture is a MsgHandler that captures leap second messages
// received during GPS configuration.
type leapSecondCapture struct {
	gpsprot.DefaultHandler
	msg *gpsprot.LeapSecondMsg
}

func (h *leapSecondCapture) LeapSecond(msg *gpsprot.LeapSecondMsg, _ time.Time) {
	h.msg = msg
}

func (h *leapSecondCapture) logLeapSecond(lg *slog.Logger) {
	if h.msg != nil {
		lsdStr := h.msg.Date().Format("2006-01-02")
		lg.Info("leap second information received from GPS", "date", lsdStr, "utcOffBefore", h.msg.UTCOffBefore, "utcOffAfter", h.msg.UTCOffAfter)
	}
}

func createConfigTarget(lg *slog.Logger, cfg *Config, speed int, timePulseEnabled bool) (*gpsprot.ConfigTarget, error) {
	var cf cfgFeatures
	if timePulseEnabled {
		cf |= cfgTimePulse
	}
	if cfg.Log.Track || len(cfg.HTTP) > 0 {
		cf |= cfgPosition
	}
	if cfg.httpWantsSatellites() {
		cf |= cfgSatellites
	}
	gct, err := cfg.GPS.target(speed, cf)
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
