package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"sync"

	"github.com/jclark/gps4ptp/internal/bcast"
	"github.com/jclark/gps4ptp/internal/gpscfg"
	"github.com/jclark/gps4ptp/internal/logctx"
	"github.com/jclark/gps4ptp/internal/mon"
	"github.com/jclark/gps4ptp/internal/pmc"
	"github.com/jclark/gps4ptp/internal/scan"
	"github.com/jclark/gps4ptp/internal/serio"
	"github.com/jclark/gps4ptp/internal/sse"

	"golang.org/x/sys/unix"
)

func main() {
	var configFile string
	var debugEnable bool
	var sdLog bool
	var showVersion bool
	var inputLogFile string

	flag.StringVar(&configFile, "f", defaultConfigFile, "configuration file")
	flag.StringVar(&inputLogFile, "inputLogFile", "", "input log file")
	flag.BoolVar(&showVersion, "version", false, "show version information")
	flag.BoolVar(&debugEnable, "debug", false, "log debugging information")
	flag.BoolVar(&sdLog, "sdlog", false, "log to stdout with priorities in systemd-compatible format")
	flag.Parse()
	if showVersion {
		fmt.Println(versionInfo())
		os.Exit(0)
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
	ctx := logctx.NewContext(context.Background(), lg)
	ctx, cancel := cancelOnSignal(ctx)
	err := run(ctx, cancel, configFile, inputLogFile)
	if err != nil {
		fmt.Fprintln(os.Stderr, os.Args[0]+":", err)
		s := configErrorDetail(err)
		if s != "" {
			fmt.Fprintln(os.Stderr, s)
		}
		os.Exit(1)
	}
}

func run(ctx context.Context, cancel context.CancelFunc, cfgFile string, inputLogFile string) error {
	cfg, err := loadConfig(cfgFile)
	if err != nil {
		return err
	}

	var inLog io.Writer
	if inputLogFile != "" {
		f, err := os.Create(inputLogFile)
		if err != nil {
			return err
		}
		inLog = f
		defer f.Close()
	}

	clk, err := openExttsClock(cfg.PHC)
	if err != nil {
		return err
	}
	lg := logctx.FromContext(ctx)
	lg.Info("selected PTP hardware clock", "path", clk.Path())

	defer func() {
		clk.Close()
		lg.Debug("closed the PHC", "interface", cfg.PHC.Interface)
	}()
	t, err := serio.OpenTerm(cfg.Serial.Device, cfg.Serial.Speed)
	if err != nil {
		return err
	}

	defer func() {
		serialDev := cfg.Serial.Device
		lg.Debug("restoring the serial port settings", "path", serialDev)
		e := t.Restore()
		if e != nil {
			lg.Error("error while restoring the serial port settings", e, "path", serialDev)
		} else {
			lg.Debug("successfully restored the serial port settings", "path", serialDev)
		}
		lg.Debug("closing the serial port", "path", serialDev)
		t.Close()
		lg.Debug("successfully closed the serial port", "path", serialDev)
	}()

	lg.Debug("detecting kind of serial port", "devKind", t.DevKind())

	scanner := serio.NewScanner(t)
	var wg sync.WaitGroup

	fCh := startScan(ctx, &wg, scanner)

	fb := startBcast(ctx, lg, &wg, fCh)

	var sseCh chan sse.Event
	var eb *bcast.Bcast[sse.Event]
	if len(cfg.HTTP) > 0 {
		sseCh = make(chan sse.Event, 1)
		eb = startBcast(ctx, lg, &wg, sseCh)
	}
	// Shut down the broadcast goroutines when the context is cancelled.
	waitGroupGo(&wg, func() {
		<-ctx.Done()
		fb.Close()
		if eb != nil {
			eb.Close()
		}
	})
	fCh = fb.Subscribe()
	defer func() {
		// Avoid calling wg.Wait() if we panic
		if r := recover(); r != nil {
			panic(r)
		}
		// startScan starts a goroutine sending to fCh.
		// We need to wait for the goroutine to close fCh, before calling port.Restore/port.Close.
		// Otherwise, there is a possibility of reading from a file descriptor that
		// is no longer valid (and so might refer to something else).
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
	var _ gpscfg.SerialError = serio.TermError{}
	initData, err := gpscfg.Configure(ctx, lg, fCh, t)
	if err != nil {
		return err
	}
	if ctx.Err() != nil {
		return nil
	}

	err = startTCP(ctx, lg, &wg, cfg.TCP, fb, t)
	if err != nil {
		return err
	}

	if eb != nil {
		initEvent, err := sse.Make("init", initData)
		if err != nil {
			return err
		}
		err = startHTTP(ctx, lg, &wg, cfg.HTTP, eb, initEvent)
		if err != nil {
			return err
		}
	}

	gmUpdateCh := make(chan mon.GrandmasterUpdateRequest)
	var pmcClient *pmc.Client
	switch cfg.PTP.Impl {
	case PTPImplNone:
		gmUpdateCh = nil
	case PTPImplPTP4L:
		pmcClient, err = cfg.PTP.NewClient()
		if err != nil {
			return err
		}
	}
	s, err := NewSyncRunner(lg, clk, cfg, gmUpdateCh, sseCh, inLog)
	if err != nil {
		return err
	}

	tsCh, edges, err := StartPPS(ctx, clk, cfg.PHC)
	if err != nil {
		return err
	}
	lg.Info("started external timestamping", "edges", edges)
	if pmcClient != nil {
		waitGroupGo(&wg, func() { mon.PTP4LWorker(ctx, pmcClient, gmUpdateCh, lg) })
	}
	// the SyncRunner assumes responsibility for closing the sseCh
	sseCh = nil
	waitGroupGo(&wg, func() {
		s.run(tsCh, fCh)
		close(gmUpdateCh)
	})

	return nil
}

func waitGroupGo(wg *sync.WaitGroup, f func()) {
	wg.Add(1)
	go func() {
		defer wg.Done()
		f()
	}()
}

func cancelOnSignal(ctx context.Context) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(ctx)
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, unix.SIGTERM)
	go func() {
		<-sig
		logctx.FromContext(ctx).Debug("received signal, initiating cancellation")
		cancel()
	}()
	return ctx, cancel
}

func startScan(ctx context.Context, wg *sync.WaitGroup, scanner *scan.Scanner) <-chan scan.Packet {
	msg := make(chan scan.Packet, 1)
	waitGroupGo(wg, func() { serio.ScanWorker(ctx, scanner, msg) })
	return msg
}

func startBcast[T any](ctx context.Context, lg *slog.Logger, wg *sync.WaitGroup, msg <-chan T) *bcast.Bcast[T] {
	b := bcast.New(msg)
	waitGroupGo(wg, func() { b.Run(ctx, lg) })
	return b
}
