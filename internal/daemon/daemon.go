package daemon

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/jclark/satpulse/internal/bcast"
	"github.com/jclark/satpulse/internal/cmd"
	"github.com/jclark/satpulse/internal/gpscfg"
	"github.com/jclark/satpulse/internal/gpsevent"
	"github.com/jclark/satpulse/internal/gpsio"
	"github.com/jclark/satpulse/internal/gpsprot"
	"github.com/jclark/satpulse/internal/mon"
	"github.com/jclark/satpulse/internal/obs"
	"github.com/jclark/satpulse/internal/phc"
	"github.com/jclark/satpulse/internal/promobs"
	"github.com/jclark/satpulse/internal/proxy"
	"github.com/jclark/satpulse/internal/scan"
	"github.com/jclark/satpulse/internal/servo"
	"github.com/jclark/satpulse/internal/sse"
	"github.com/jclark/satpulse/internal/sseobs"
	"github.com/jclark/satpulse/internal/ts"
)

func Cmd(progName string, args []string) {
	vars, msg, err := parseFlags(progName, args)
	if vars == nil {
		exitCode := 0
		if err != nil {
			cmd.ErrPrintln(progName, err)
			if msg != "" {
				exitCode = 2
			} else {
				exitCode = 1
			}
		}
		fmt.Fprint(os.Stderr, msg)
		os.Exit(exitCode)
	}
	cfg, err := LoadConfig(vars.configFiles...)
	if err != nil {
		cmd.ErrPrintln(progName, err)
		s := configErrorDetail(err)
		if s != "" {
			fmt.Fprintln(os.Stderr, s)
		}
		os.Exit(1)
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
	ctx := context.Background()
	ctx, cancel := cmd.CancelOnSignal(ctx, lg)
	err = run(ctx, lg, cancel, cfg)
	if err != nil {
		cmd.ErrPrintln(progName, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, lg *slog.Logger, cancel context.CancelFunc, cfg *Config) error {
	tStart := time.Now()
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
		lg.Info("no interface specified, running in GPS only mode")
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
	pLog, lf, err := gpsio.LogPackets(lg, &wg, cfg.Log.PacketPath(cfg.Serial.Device, gpsio.PacketLogExtension))
	if err != nil {
		return err
	}
	if lf != nil {
		defer lf.Close(lg)
	}
	if pLog != nil {
		conn.SetPacketLog(pLog)
	}
	pCh := startScan(ctx, lg, &wg, conn, pLog)

	pb := startBcast(ctx, lg, &wg, pCh)

	var sseCh chan sse.Event
	var eb *bcast.Bcast[sse.Event]
	if len(cfg.HTTP) > 0 {
		sseCh = make(chan sse.Event, 1)
		eb = startBcast(ctx, lg, &wg, sseCh)
	}
	// Shut down the broadcast goroutines when the context is cancelled.
	cmd.WaitGroupGo(&wg, func() {
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

	pktProcs, err := cfg.GPS.CreatePacketProcessors()
	if err != nil {
		return err
	}
	// Let the compiler check that TermError implements the SerialError interface
	// gpsInit relies on this
	var _ gpscfg.SerialError = gpsio.TermError{}
	var tpFlags gpsTimePulseFlags
	if clk != nil {
		tpFlags |= gpsTimePulseEnable
		if phcFlags.Edges() != 1 {
			tpFlags |= gpsTimePulseGetWidth
		}
	}
	gct, pulseWidth, err := createConfigTarget(lg, cfg, conn.Speed(), tpFlags)
	if err != nil {
		return err
	}
	gcfg, err := gpscfg.Configure(ctx, lg, pktProcs, gct, pCh, conn)
	if err != nil {
		if errors.Is(err, gpscfg.ErrNoProbeResponse) {
			lg.Info(err.Error())
		} else {
			return err
		}
	}
	if ctx.Err() != nil {
		return nil
	}

	err = proxy.Start(ctx, lg, &wg, cfg.Proxy, pb, conn)
	if err != nil {
		return err
	}

	promObs := newPrometheusObserver(cfg)
	sseObs := newSSEObserver(cfg, sseCh, lg, gcfg)
	if eb != nil {
		err = startHTTP(ctx, lg, &wg, cfg.HTTP, eb, sseObs, promObs)
		if err != nil {
			return err
		}
	}

	var (
		gm         *mon.Grandmaster
		gmUpdateCh <-chan mon.GrandmasterUpdateRequest
	)
	pmcClient, err := cfg.PTP.NewClient()
	if err != nil {
		return err
	}
	if pmcClient != nil {
		gm, gmUpdateCh = mon.NewGrandmaster()
	}

	rc, err := cfg.NTP.NewRefClock(lg)
	var (
		rcProxy *mon.ProxyRefClock
		rcCh    <-chan mon.RefClockSample
	)
	if rc != nil {
		rcProxy, rcCh = mon.NewProxyRefClock()
	}
	var tsCh <-chan ts.Event
	if clk != nil {
		tsCh, err = ts.StartWorker(ctx, clk, lg)
		if err != nil {
			return err
		}
	}
	if pw, ok := gcfg.ConfigProps.GetTimePulseWidth(); ok {
		pulseWidth = pw
	}

	d, err := NewDispatcher(lg, pktProcs, clk, pulseWidth, cfg, gm, rcProxy, combineObservers(promObs, sseObs), tStart)
	if err != nil {
		return err
	}

	if pmcClient != nil {
		cmd.WaitGroupGo(&wg, func() { mon.PTP4LWorker(pmcClient, gmUpdateCh, lg) })
	}
	if rc != nil {
		cmd.WaitGroupGo(&wg, func() { mon.RefClockWorker(rc, rcCh, lg) })
	}
	// the SyncRunner assumes responsibility for closing the sseCh
	sseCh = nil
	ls := gcfg.LeapSecond
	cmd.WaitGroupGo(&wg, func() {
		if ls != nil {
			d.LeapSecond(ls, time.Time{})
		}
		d.Run(tsCh, pCh)
		if rcProxy != nil {
			rcProxy.Close()
		}
	})

	return nil
}

func NewDispatcher(lg *slog.Logger, pktProcs map[gpsprot.Tag]gpsprot.PacketProcessor, clk *ts.Clock, pulseWidth time.Duration, cfg *Config, gm *mon.Grandmaster, rc *mon.ProxyRefClock, obs obs.Observer, tStart time.Time) (*gpsevent.Dispatcher, error) {
	ls := cfg.LeapSecond.leapSecond()
	var m *mon.Monitor
	var driverFlags phc.DriverFlags
	if clk != nil {
		servo, err := servo.New(clk, lg)
		if err != nil {
			return nil, err
		}
		m, err = mon.NewMonitor(servo, lg, mon.MonitorConfig{
			LeapSecond:    ls,
			Sampler:       obs,
			RefClock:      rc,
			Grandmaster:   gm,
			LogInterval:   cfg.Log.Interval,
			ClockLogPath:  cfg.Log.ClockPath(clk.IfName(), mon.ClockLogExtension),
			ClockAccuracy: time.Duration(cfg.PTP.ClockAccuracy),
		})
		if err != nil {
			return nil, err
		}
		driverFlags = clk.DriverFlags
	}
	eventLogPath := cfg.Log.EventPath(cfg.Serial.Device, gpsevent.LogExtension)
	return gpsevent.NewDispatcher(lg, pktProcs, m, ls, driverFlags, pulseWidth, obs, eventLogPath, tStart)
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

// combineObservers combines individual observers into appropriate single observer
func combineObservers(promObs *promobs.PrometheusObserver, sseObs obs.Observer) obs.Observer {
	if promObs != nil && sseObs != nil {
		return obs.NewMultiObserver(promObs, sseObs)
	} else if promObs != nil {
		return promObs
	} else if sseObs != nil {
		return sseObs
	} else {
		return &obs.DefaultObserver{}
	}
}

func startScan(ctx context.Context, lg *slog.Logger, wg *sync.WaitGroup, conn gpsio.Conn, pLog *gpsio.PacketLog) <-chan scan.Packet {
	msg := make(chan scan.Packet, 1)
	cmd.WaitGroupGo(wg, func() { gpsio.Scan(ctx, lg, conn, msg, pLog) })
	return msg
}

func startBcast[T any](ctx context.Context, lg *slog.Logger, wg *sync.WaitGroup, msg <-chan T) *bcast.Bcast[T] {
	b := bcast.New(msg)
	cmd.WaitGroupGo(wg, func() { b.Run(ctx, lg) })
	return b
}

func createConfigTarget(lg *slog.Logger, cfg *Config, speed int, tpFlags gpsTimePulseFlags) (*gpsprot.ConfigTarget, time.Duration, error) {
	httpWantsSatellites := cfg.httpWantsSatellites()
	gct, pulseWidth, err := cfg.GPS.target(speed, httpWantsSatellites, tpFlags)
	lg.Debug("GPS configure input", "target", gct)
	if err != nil {
		return nil, 0, err
	}
	if httpWantsSatellites && gct.Opts.SatsMsg.Get() == gpsprot.SatsMsgNone {
		lg.Warn("satellites output will not be enabled, because serial speed is too low",
			"speed", speed, "minSpeedForSatellitesOutput", minSpeedSatellitesOutput)
	}
	return gct, pulseWidth, nil
}
