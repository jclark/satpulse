package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"sync"

	"github.com/jclark/satpulse/internal/gpscfg"
	"github.com/jclark/satpulse/internal/gpsmsg"
	"github.com/jclark/satpulse/internal/scan"
	"github.com/jclark/satpulse/internal/serio"

	"github.com/spf13/pflag"

	"golang.org/x/sys/unix"
)

func main() {
	var verboseLevel int
	var speed intFlag
	var help bool
	var sane bool
	config := &gpsmsg.Config{}

	flags := pflag.NewFlagSet("gpsconfig", pflag.ContinueOnError)
	flags.SetInterspersed(false)

	flags.CountVarP(&verboseLevel, "verbose", "v", "increase verbosity")
	flags.VarP(&speed, "speed", "s", "serial device `baud-rate`")
	flags.BoolVarP(&help, "help", "h", false, "show help")
	flags.BoolVarP(&sane, "sane", "S", false, "configure the receiver defaults that are sane for timing")
	err := flags.Parse(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, os.Args[0]+":", err)
		os.Exit(2)
	}
	if help {
		usage(flags)
		os.Exit(0)
	}

	if flags.NArg() != 1 {
		fmt.Fprintln(os.Stderr, os.Args[0]+": must specify a serial device")
		usage(flags)
		os.Exit(2)
	}
	device := flags.Arg(0)

	if sane {
		config.SetSane()
		gpsmsg.CfgNMEAEnabled.Set(config, true)
	}
	level := slog.LevelWarn
	if verboseLevel == 1 {
		level = slog.LevelInfo
	} else if verboseLevel > 1 {
		level = slog.LevelDebug
	}

	handler := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level})

	lg := slog.New(handler)
	slog.SetDefault(lg)
	ctx := context.Background()
	ctx, cancel := cancelOnSignal(ctx, lg)
	err = run(ctx, lg, cancel, config, device, speed.value)
	if err != nil {
		fmt.Fprintln(os.Stderr, os.Args[0]+":", err)
		os.Exit(1)
	}
}

func usage(flags *pflag.FlagSet) {
	fmt.Fprintln(os.Stderr, "Usage:", os.Args[0], "[options] serial-device")
	fmt.Fprintln(os.Stderr, "Options:")
	flags.PrintDefaults()
}

func run(ctx context.Context, lg *slog.Logger, cancel context.CancelFunc, config *gpsmsg.Config, serialDev string, speed *int) error {

	t, err := serio.OpenTerm(serialDev, speed)
	if err != nil {
		return err
	}

	defer func() {
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

	// Let the compiler check that TermError implements the SerialError interface
	// gpsInit relies on this
	var _ gpscfg.SerialError = serio.TermError{}
	info, err := gpscfg.Configure(ctx, lg, config, pCh, t)
	if err == nil {
		fmt.Printf("set config to: %s\n", fmt.Sprint(info.Config))
	}

	lg.Debug("about to wait")

	// stop the scan worker
	cancel()
	wg.Wait()
	return err
}

func startScan(ctx context.Context, lg *slog.Logger, wg *sync.WaitGroup, scanner *scan.Scanner) <-chan scan.Packet {
	msg := make(chan scan.Packet, 1)
	waitGroupGo(wg, func() { serio.ScanWorker(ctx, lg, scanner, msg) })
	return msg
}

func waitGroupGo(wg *sync.WaitGroup, f func()) {
	wg.Add(1)
	go func() {
		defer wg.Done()
		f()
	}()
}

func cancelOnSignal(ctx context.Context, lg *slog.Logger) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(ctx)
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, unix.SIGTERM)
	go func() {
		<-sig
		lg.Debug("received signal, initiating cancellation")
		cancel()
	}()
	return ctx, cancel
}

type intFlag struct {
	value *int
}

func (i *intFlag) String() string {
	if i.value == nil {
		return ""
	}
	return strconv.Itoa(*i.value)
}

func (i *intFlag) Type() string {
	return "int"
}

func (i *intFlag) Set(s string) error {
	v, err := strconv.Atoi(s)
	if err != nil {
		return err
	}
	i.value = &v
	return nil
}
