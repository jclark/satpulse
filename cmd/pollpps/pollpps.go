//go:build !windows

// pollpps measures PPS detection on a serial modem-control input.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"sync"
	"time"

	"github.com/jclark/satpulse/gps/lib/term"
)

const diagnosticReadTimeout = 100 * time.Millisecond

func main() {
	useD2XX := flag.Bool("d2xx", false, "wait for PPS using the FTDI D2XX backend")
	speed := flag.Int("speed", 9600, "serial speed in bits per second")
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "usage: %s [options] device\n", os.Args[0])
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(2)
	}
	if err := run(flag.Arg(0), *speed, *useD2XX); err != nil {
		log.Fatal(err)
	}
}

func run(device string, speed int, useD2XX bool) error {
	if !term.IsValidSpeed(speed) {
		return fmt.Errorf("non-standard serial speed %d is not supported", speed)
	}
	t, err := term.Open(device, term.RawMode, term.Local, term.NoParity,
		term.NoFlowControl, term.ReadTimeout(diagnosticReadTimeout), term.Speed(speed))
	if err != nil {
		return err
	}
	defer func() {
		t.Restore()
		t.Close()
	}()
	if err := t.Flush(); err != nil {
		return err
	}
	wt, canWait := t.(waitTerm)
	if useD2XX && !canWait {
		return errors.New("D2XX backend is not available for this device")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	fmt.Printf("Monitoring PPS on CTS of %s at %d bps using %s\n", device, t.Speed(), backendName(useD2XX))
	if useD2XX {
		return monitorD2XX(ctx, wt)
	}
	return monitorPolling(ctx, t)
}

// waitTerm is a terminal that can also block until a modem control pin
// changes, which only the D2XX backend can do.
type waitTerm interface {
	term.Term
	term.ModemControlPinWaiter
}

func backendName(useD2XX bool) string {
	if useD2XX {
		return "D2XX"
	}
	return "polling"
}

func monitorD2XX(ctx context.Context, t waitTerm) error {
	ctx, cancel := context.WithCancel(ctx)
	readErr := make(chan error, 1)
	var wg sync.WaitGroup
	wg.Go(func() { drainD2XX(ctx, t, readErr) })
	defer func() {
		cancel()
		t.CancelModemControlPinWait()
		wg.Wait()
	}()
	wg.Go(func() {
		<-ctx.Done()
		t.CancelModemControlPinWait()
	})
	status, err := t.ModemControlPinState()
	if err != nil {
		return err
	}
	lastCTS := status.Asserted(term.ModemCTS)
	count := 0
	for {
		at, err := t.WaitModemControlPinChange(term.ModemCTS)
		if err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return nil
		case err := <-readErr:
			return err
		default:
		}
		status, err = t.ModemControlPinState()
		if err != nil {
			return err
		}
		cts := status.Asserted(term.ModemCTS)
		if lastCTS && !cts {
			count++
			printPPS(count, at)
		}
		lastCTS = cts
	}
}

func drainD2XX(ctx context.Context, t waitTerm, errCh chan<- error) {
	buf := make([]byte, 4096)
	for {
		_, err := t.Read(buf)
		if err == nil || errors.Is(err, os.ErrDeadlineExceeded) {
			select {
			case <-ctx.Done():
				return
			default:
				continue
			}
		}
		if errors.Is(err, io.EOF) && ctx.Err() != nil {
			return
		}
		select {
		case errCh <- err:
		default:
		}
		t.CancelModemControlPinWait()
		return
	}
}

func monitorPolling(ctx context.Context, t term.Term) error {
	ticker := time.NewTicker(100 * time.Microsecond)
	defer ticker.Stop()
	status, err := t.ModemControlPinState()
	if err != nil {
		return err
	}
	lastCTS := status.Asserted(term.ModemCTS)
	count := 0
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
		status, err = t.ModemControlPinState()
		if err != nil {
			return err
		}
		cts := status.Asserted(term.ModemCTS)
		if lastCTS && !cts {
			count++
			printPPS(count, time.Now())
		}
		lastCTS = cts
	}
}

func printPPS(count int, at time.Time) {
	fmt.Printf("PPS #%d at %s (%d.%09d)\n", count,
		at.Format("15:04:05.000000000"), at.Unix(), at.Nanosecond())
}
