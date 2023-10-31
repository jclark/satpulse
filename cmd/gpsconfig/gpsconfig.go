package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"sync"

	"github.com/jclark/gps4ptp/internal/gpscfg"
	"github.com/jclark/gps4ptp/internal/scan"
	"github.com/jclark/gps4ptp/internal/serio"

	"github.com/spf13/pflag"

	"golang.org/x/sys/unix"
)

func main() {
	var debugEnable bool
	var device string
	var speed intFlag

	pflag.BoolVar(&debugEnable, "debug", false, "log debugging information")
	pflag.StringVar(&device, "device", "/dev/ttyS0", "serial device")
	pflag.Var(&speed, "speed", "serial port speed")

	pflag.Parse()

	level := slog.LevelInfo
	if debugEnable {
		level = slog.LevelDebug
	}

	handler := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level})

	lg := slog.New(handler)
	slog.SetDefault(lg)
	ctx := context.Background()
	ctx, cancel := cancelOnSignal(ctx, lg)
	err := run(ctx, lg, cancel, device, speed.value)
	if err != nil {
		fmt.Fprintln(os.Stderr, os.Args[0]+":", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, lg *slog.Logger, cancel context.CancelFunc, serialDev string, speed *int) error {

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
	_, err = gpscfg.Configure(ctx, lg, pCh, t)

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
