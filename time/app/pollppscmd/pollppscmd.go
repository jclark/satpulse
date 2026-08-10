// Package pollppscmd implements pollpps, an internal tool that runs the
// serialpps polling backend against a serial port's CTS pin and prints the
// edges it detects, or times the modem status read that paces the polling.
// The GPS PPS output should be connected to the CTS pin of a USB to TTL
// adapter.
package pollppscmd

import (
	"context"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"slices"
	"time"

	"github.com/jclark/satpulse/gps/app/cmd"
	"github.com/jclark/satpulse/gps/app/gpsio"
	"github.com/jclark/satpulse/time/internal/serialpps"
	"github.com/spf13/pflag"
)

// Main runs the pollpps command.
func Main() {
	var help bool
	var showVersion bool
	var ioctlTime bool
	var speed int

	flags := pflag.NewFlagSet("pollpps", pflag.ContinueOnError)

	flags.BoolVarP(&help, "help", "h", false, "show help")
	flags.BoolVarP(&showVersion, "version", "V", false, "show version information")
	flags.BoolVarP(&ioctlTime, "ioctltime", "i", false, "time back-to-back modem status reads and exit")
	flags.IntVarP(&speed, "speed", "s", 0, "set the baud rate so serial data flows while measuring (0 leaves it unchanged)")

	err := flags.Parse(os.Args[1:])
	progName := os.Args[0]
	if err != nil {
		cmd.ErrPrintln(progName, err)
		usage(progName, flags)
		os.Exit(2)
	}
	if showVersion {
		fmt.Fprintln(os.Stderr, cmd.VersionInfo())
		os.Exit(0)
	}
	if help {
		usage(progName, flags)
		os.Exit(0)
	}
	if flags.NArg() != 1 {
		usage(progName, flags)
		os.Exit(2)
	}

	device := flags.Args()[0]

	// Use the daemon's serial connection so reads have the same timeout and
	// shutdown synchronization as the scan worker.
	t, _, err := gpsio.OpenSerial(device, speed)
	if err != nil {
		log.Fatalf("Failed to open %s: %v", device, err)
	}
	defer t.Close()
	done := drainInput(t)
	defer func() {
		t.Stop()
		<-done
	}()
	if ioctlTime {
		timeIoctl(t)
		return
	}
	fmt.Printf("Monitoring PPS on CTS pin of %s\n", device)
	monitor(t)
}

// monitor runs the polling backend, echoing its debug log to stderr so
// settling, the window equilibrium, and the miss rate are visible, and
// prints each published edge.
func monitor(t *gpsio.SerialConn) {
	lg := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	edges := make(chan serialpps.Edge)
	errCh := make(chan error, 1)
	go func() {
		errCh <- serialpps.Poll(ctx, t, serialpps.Wiring{Pin: gpsio.ModemCTS}, edges, lg)
	}()
	ppsCount := 0
	for {
		select {
		case e := <-edges:
			ppsCount++
			fmt.Printf("PPS #%d at %s (%d.%09d)\n",
				ppsCount,
				e.Wall.Format("15:04:05.000000000"),
				e.Wall.Unix(),
				e.Wall.Nanosecond())
		case err := <-errCh:
			if err != nil && !errors.Is(err, context.Canceled) {
				log.Fatalf("Poll failed: %v", err)
			}
			fmt.Printf("\nReceived interrupt, shutting down...\n")
			return
		}
	}
}

func timeIoctl(t *gpsio.SerialConn) {
	const n = 2000
	ds, err := serialpps.TimeStateReads(t, n)
	if err != nil {
		log.Fatalf("Error reading modem status: %v", err)
	}
	slices.Sort(ds)
	var sum time.Duration
	for _, d := range ds {
		sum += d
	}
	fmt.Printf("modem status read over %d calls: min=%v median=%v mean=%v p90=%v max=%v\n",
		n, ds[0], ds[n/2], sum/n, ds[n*9/10], ds[n-1])
}

func drainInput(t *gpsio.SerialConn) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		b := make([]byte, 4096)
		for {
			if _, err := t.Read(b); errors.Is(err, os.ErrDeadlineExceeded) {
				continue
			} else if err != nil {
				return
			}
		}
	}()
	return done
}

func usage(progName string, flags *pflag.FlagSet) {
	fmt.Fprintln(os.Stderr, "Usage:", progName, "[-i|--ioctltime] [-s|--speed <baud>] <device>")
	fmt.Fprintln(os.Stderr, "Options:")
	flags.PrintDefaults()
}
