package daemon

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/jclark/satpulse/internal/bcast"
	"github.com/jclark/satpulse/internal/cmd"
	"github.com/jclark/satpulse/internal/cmd/daemon/proxy"
	"github.com/jclark/satpulse/internal/gpscfg"
	"github.com/jclark/satpulse/internal/gpsio"
	"github.com/jclark/satpulse/internal/gpsprot"
	"github.com/jclark/satpulse/internal/mon"
	"github.com/jclark/satpulse/internal/phc"
	"github.com/jclark/satpulse/internal/pmc"
	"github.com/jclark/satpulse/internal/scan"
	"github.com/jclark/satpulse/internal/sse"
	"github.com/jclark/satpulse/internal/ubx"
)

func Cmd(progName string, args []string) {
	var configFile string
	var debugEnable bool
	var sdLog bool
	var showVersion bool
	var inputLogFile string
	var wait bool

	flags := flag.NewFlagSet("config", flag.ExitOnError)

	flags.StringVar(&configFile, "f", defaultConfigFile, "configuration file")
	flags.StringVar(&inputLogFile, "inputLogFile", "", "input log file")
	flags.BoolVar(&showVersion, "version", false, "show version information")
	flags.BoolVar(&debugEnable, "debug", false, "log debugging information")
	flags.BoolVar(&sdLog, "sdlog", false, "log to stdout with priorities in systemd-compatible format")
	flags.BoolVar(&wait, "wait", false, "wait for the network interface to be ready")
	flags.Parse(args)
	if showVersion {
		fmt.Println(cmd.VersionInfo())
		os.Exit(0)
	}
	cfg, err := LoadConfig(configFile)
	if err != nil {
		cmd.ErrPrintln(progName, err)
		s := configErrorDetail(err)
		if s != "" {
			fmt.Fprintln(os.Stderr, s)
		}
		os.Exit(1)
	}
	if wait {
		cfg.PHC.Wait = true
	}
	level := slog.LevelInfo
	if debugEnable {
		level = slog.LevelDebug
	}
	var handler slog.Handler
	if sdLog {
		handler = NewSdHandler(level, os.Stdout)
	} else {
		handler = slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: level})
	}
	lg := slog.New(handler)
	slog.SetDefault(lg)
	ctx := context.Background()
	ctx, cancel := cmd.CancelOnSignal(ctx, lg)
	err = run(ctx, lg, cancel, cfg, inputLogFile)
	if err != nil {
		cmd.ErrPrintln(progName, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, lg *slog.Logger, cancel context.CancelFunc, cfg *Config, inputLogFile string) error {
	var inLog io.Writer
	if inputLogFile != "" {
		f, err := os.Create(inputLogFile)
		if err != nil {
			return err
		}
		inLog = f
		defer f.Close()
	}

	clk, phcFlags, err := openExttsClock(lg, cfg.PHC)
	if err != nil {
		return err
	}
	lg.Info("selected PTP hardware clock", "path", clk.Path(),
		"known", phcFlags&phc.DriverKnown != 0,
		"bothEdges", phcFlags&phc.DriverBothEdges != 0,
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
	gmap, gopts, err := gpsConfig(cfg.Receiver)
	lg.Debug("GPS configure input", "map", gmap, "opts", gopts)
	if err != nil {
		return err
	}
	gcfg, err := gpscfg.Configure(ctx, lg, gmap, gopts, pCh, conn)
	if err != nil {
		return err
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
		gm *mon.Grandmaster
		gmUpdateCh <-chan mon.GrandmasterUpdateRequest
		pmcClient *pmc.Client
	)
	switch cfg.PTP.Impl {
	case PTPImplNone:
		// do nothing 
	case PTPImplPTP4L:
		pmcClient, err = cfg.PTP.NewClient()
		if err != nil {
			return err
		}
		gm, gmUpdateCh = mon.NewGrandmaster()
	}

	tsCh, edges, err := StartPPS(ctx, clk, cfg.PHC)
	if err != nil {
		return err
	}
	lg.Info("started external timestamping", "edges", edges)

	pulseWidth, ok := gpsprot.CfgTimePulseWidth.Get(gcfg.ConfigMap)
	if !ok {
		pulseWidth = 0
	}
		
	s, err := NewSyncRunner(lg, clk, phcFlags.SetEdges(edges), pulseWidth, cfg, gm, sseCh, inLog)
	if err != nil {
		return err
	}

	if pmcClient != nil {
		cmd.WaitGroupGo(&wg, func() { mon.PTP4LWorker(ctx, pmcClient, gmUpdateCh, lg) })
	}
	// the SyncRunner assumes responsibility for closing the sseCh
	sseCh = nil
	ls := gcfg.LeapSecond
	cmd.WaitGroupGo(&wg, func() {
		if ls != nil {
			s.LeapSecond(ls, time.Time{})
		}
		s.run(tsCh, pCh)
		if gm != nil {
			gm.Close()
		}
	})

	return nil
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
