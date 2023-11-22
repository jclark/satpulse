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
	"github.com/jclark/satpulse/internal/mon"
	"github.com/jclark/satpulse/internal/pmc"
	"github.com/jclark/satpulse/internal/scan"
	"github.com/jclark/satpulse/internal/serio"
	"github.com/jclark/satpulse/internal/sse"
	"github.com/jclark/satpulse/internal/ubx"
)

func Cmd(progName string, args []string) {
	var configFile string
	var debugEnable bool
	var sdLog bool
	var showVersion bool
	var inputLogFile string

	flags := flag.NewFlagSet("config", flag.ExitOnError)

	flags.StringVar(&configFile, "f", defaultConfigFile, "configuration file")
	flags.StringVar(&inputLogFile, "inputLogFile", "", "input log file")
	flags.BoolVar(&showVersion, "version", false, "show version information")
	flags.BoolVar(&debugEnable, "debug", false, "log debugging information")
	flags.BoolVar(&sdLog, "sdlog", false, "log to stdout with priorities in systemd-compatible format")
	flags.Parse(args)
	if showVersion {
		fmt.Println(cmd.VersionInfo())
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
	ctx := context.Background()
	ctx, cancel := cmd.CancelOnSignal(ctx, lg)
	err := run(ctx, lg, cancel, configFile, inputLogFile)
	if err != nil {
		cmd.ErrPrintln(progName, err)
		s := configErrorDetail(err)
		if s != "" {
			fmt.Fprintln(os.Stderr, s)
		}
		os.Exit(1)
	}
}

func run(ctx context.Context, lg *slog.Logger, cancel context.CancelFunc, cfgFile string, inputLogFile string) error {
	cfg, err := LoadConfig(cfgFile)
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

	pCh := startScan(ctx, lg, &wg, scanner)

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
		// startScan starts a goroutine sending to pCh.
		// We need to wait for the goroutine to close pCh, before calling port.Restore/port.Close.
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
	gcfg, err := gpscfg.Configure(ctx, lg, gpscfg.RequiredConfig(), gpscfg.RequiredOptions(), pCh, t)
	if err != nil {
		return err
	}
	if ctx.Err() != nil {
		return nil
	}

	err = proxy.Start(ctx, lg, &wg, cfg.Proxy, pb, t)
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
		close(gmUpdateCh)
	})

	return nil
}

type InitData struct {
	Version *ubx.Version `json:"version,omitempty"`
}

func newInitData(r *gpscfg.Result) *InitData {
	return &InitData{Version: r.Version}
}

func startScan(ctx context.Context, lg *slog.Logger, wg *sync.WaitGroup, scanner *scan.Scanner) <-chan scan.Packet {
	msg := make(chan scan.Packet, 1)
	cmd.WaitGroupGo(wg, func() { serio.ScanWorker(ctx, lg, scanner, msg) })
	return msg
}

func startBcast[T any](ctx context.Context, lg *slog.Logger, wg *sync.WaitGroup, msg <-chan T) *bcast.Bcast[T] {
	b := bcast.New(msg)
	cmd.WaitGroupGo(wg, func() { b.Run(ctx, lg) })
	return b
}
