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
	"github.com/jclark/satpulse/internal/phc"
	"github.com/jclark/satpulse/internal/proxy"
	"github.com/jclark/satpulse/internal/scan"
	"github.com/jclark/satpulse/internal/servo"
	"github.com/jclark/satpulse/internal/sse"
	"github.com/jclark/satpulse/internal/ts"
	"github.com/jclark/satpulse/internal/ubx"
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
	clk, phcFlags, err := cfg.PHC.OpenClock(lg)
	if err != nil {
		return err
	}
	lg.Info("selected PTP hardware clock", "path", clk.Path(),
		"known", phcFlags&phc.DriverKnown != 0,
		"bothEdges", phcFlags&phc.DriverBothEdges != 0,
		"oneEdge", phcFlags&phc.DriverOneEdge != 0,
		"poll4Hz", phcFlags&phc.DriverPoll4Hz != 0)

	defer func() {
		clk.Close()
		lg.Debug("closed the PHC", "interface", cfg.PHC.Interface)
	}()
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

	pCh := startScan(ctx, lg, &wg, conn)

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

	// Let the compiler check that TermError implements the SerialError interface
	// gpsInit relies on this
	var _ gpscfg.SerialError = gpsio.TermError{}
	gct, err := cfg.GPS.target()
	defaultPulseWidth, err := cfg.GPS.pulseWidth()
	if err != nil {
		return fmt.Errorf("invalid pulse width in configuration file: %w", err)
	}
	if phcFlags.Edges() != 1 && defaultPulseWidth == 0 {
		gct.Get.Add(gpsprot.CfgTimePulseWidth)
	}
	lg.Debug("GPS configure input", "target", gct)
	if err != nil {
		return err
	}
	gcfg, err := gpscfg.Configure(ctx, lg, gct, pCh, conn)
	if err != nil {
		if errors.Is(err, gpscfg.ErrNoResponse) {
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

	if eb != nil {
		initEvent, err := sse.Make("init", newInitData(gcfg))
		if err != nil {
			return err
		}
		err = startHTTP(ctx, lg, &wg, cfg.HTTP, eb, initEvent)
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
	tsCh, edges, err := StartPPS(ctx, clk, cfg.PHC, lg)
	if err != nil {
		return err
	}

	pulseWidth, ok := gpsprot.CfgTimePulseWidth.Get(gcfg.ConfigMap)
	if !ok {
		pulseWidth = defaultPulseWidth
	}

	d, err := NewDispatcher(lg, clk, phcFlags.SetEdges(edges), pulseWidth, cfg, gm, rcProxy, sseCh)
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

func NewDispatcher(lg *slog.Logger, clk *ts.Clock, phcFlags phc.DriverFlags, pulseWidth time.Duration, cfg *Config, gm *mon.Grandmaster, rc *mon.ProxyRefClock, sseCh chan<- sse.Event) (*gpsevent.Dispatcher, error) {
	servo, err := servo.New(clk, lg)
	if err != nil {
		return nil, err
	}
	ls := cfg.LeapSecond.leapSecond()
	m, err := mon.NewMonitor(servo, lg, mon.MonitorConfig{
		LeapSecond:   ls,
		SSECh:        sseCh,
		RefClock:     rc,
		Grandmaster:  gm,
		LogInterval:  cfg.Log.Interval,
		ClockLogPath: cfg.Log.ClockPath(clk.Path(), mon.ClockLogExtension),
	})
	if err != nil {
		return nil, err
	}
	eventLogPath := cfg.Log.EventPath(cfg.Serial.Device, gpsevent.LogExtension)
	return gpsevent.NewDispatcher(lg, m, ls, phcFlags, pulseWidth, sseCh, eventLogPath)
}

type InitData struct {
	Version *ubx.Version `json:"version,omitempty"`
}

func newInitData(r *gpscfg.Result) *InitData {
	return &InitData{Version: r.Version}
}

func startScan(ctx context.Context, lg *slog.Logger, wg *sync.WaitGroup, conn gpsio.Conn) <-chan scan.Packet {
	msg := make(chan scan.Packet, 1)
	cmd.WaitGroupGo(wg, func() { gpsio.Scan(ctx, lg, conn, msg) })
	return msg
}

func startBcast[T any](ctx context.Context, lg *slog.Logger, wg *sync.WaitGroup, msg <-chan T) *bcast.Bcast[T] {
	b := bcast.New(msg)
	cmd.WaitGroupGo(wg, func() { b.Run(ctx, lg) })
	return b
}
