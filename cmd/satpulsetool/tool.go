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
	var help bool

	flags := pflag.NewFlagSet("satpulsetool", pflag.ContinueOnError)
	flags.SetInterspersed(false)

	flags.CountVarP(&verboseLevel, "verbose", "v", "increase verbosity")
	flags.BoolVarP(&help, "help", "h", false, "show help")
	err := flags.Parse(os.Args[1:])
	progName := os.Args[0]
	if err != nil {
		errPrintln(progName, err)
		os.Exit(2)
	}
	if help {
		usage(progName, flags)
		os.Exit(0)
	}
	if flags.NArg() == 0 {
		usage(progName, flags)
		os.Exit(2)
	}
	cmdName := flags.Arg(0)
	cmdArgs := flags.Args()[1:]

	level := slog.LevelWarn
	if verboseLevel == 1 {
		level = slog.LevelInfo
	} else if verboseLevel > 1 {
		level = slog.LevelDebug
	}
	handler := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level})
	lg := slog.New(handler)
	slog.SetDefault(lg)

	switch cmdName {
	case "config":
		err = configCmd(lg, progName, cmdName, cmdArgs)
	default:
		fmt.Fprintln(os.Stderr, os.Args[0]+": unknown command:", cmdName)
		usage(progName, flags)
		os.Exit(2)
	}

	if err != nil {
		fmt.Fprintln(os.Stderr, os.Args[0]+":", err)
		os.Exit(1)
	}
}

func configCmd(lg *slog.Logger, progName string, cmdName string, args []string) error {
	var help bool
	var sane bool
	var speed intFlag

	cm := &gpsmsg.ConfigMap{}
	flags := pflag.NewFlagSet("config", pflag.ContinueOnError)
	flags.BoolVarP(&help, "help", "h", false, "show help")

	flags.VarP(&speed, "speed", "s", "serial device `baud-rate`")
	flags.BoolVarP(&sane, "sane", "S", false, "configure the receiver defaults that are sane for timing")
	err := flags.Parse(args)
	if err != nil {
		errPrintln(progName, err)
		os.Exit(2)
	}
	const summary = "[options] serial-device"
	if flags.NArg() != 1 {
		errPrintln(progName, "config command must have argument giving serial device")
		cmdUsage(progName, cmdName, summary, flags)
		os.Exit(2)
	}
	device := flags.Arg(0)

	if sane {
		cm.SetSane()
		gpsmsg.CfgNMEAEnabled.Set(cm, false)
	}

	ctx := context.Background()
	ctx, cancel := cancelOnSignal(ctx, lg)
	return run(ctx, lg, cancel, cm, device, speed.value)
}

func usage(progName string, flags *pflag.FlagSet) {
	fmt.Fprintln(os.Stderr, "Usage:", progName, "[global-options] command [options] arg...")
	fmt.Fprintln(os.Stderr, "Commands: config")
	fmt.Fprintln(os.Stderr, "Global options:")
	flags.PrintDefaults()
}

func cmdUsage(progName, cmdName, summary string, flags *pflag.FlagSet) {
	fmt.Fprintln(os.Stderr, "Usage:", progName, cmdName, summary)
	fmt.Fprintln(os.Stderr, "Options:")
	flags.PrintDefaults()
}

func errPrintln(progName string, arg any) {
	fmt.Fprintln(os.Stderr, progName+":", arg)
}

func run(ctx context.Context, lg *slog.Logger, cancel context.CancelFunc, cm *gpsmsg.ConfigMap, serialDev string, speed *int) error {

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
	info, err := gpscfg.Configure(ctx, lg, cm, pCh, t)
	if err == nil {
		fmt.Printf("set config to: %s\n", fmt.Sprint(info.ConfigMap))
	}

	lg.Debug("about to wait")

	// stop the scan worker
	cancel()
	// need to keep reading here to avoid deadlock
	for range pCh {
	}
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
