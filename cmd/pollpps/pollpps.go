//go:build !windows

// This is an experiment for Darwin to try polling the CTS pin to detect PPS signals.
// The GPS PPS output should be connected to the CTS pin of a USB to TTL adapter.
// I have tested this with the Waveshare USB to TTL converter, which uses the FTDI FT232RNL.
package main

import (
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"slices"
	"time"

	"github.com/jclark/satpulse/gps/app/cmd"
	"github.com/jclark/satpulse/gps/app/gpsio"
	"github.com/spf13/pflag"
)

func main() {
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
	if ioctlTime {
		timeIoctl(t)
		return
	}
	fmt.Printf("Monitoring PPS on CTS pin of %s\n", device)

	var lastCTS bool
	var ppsCount int

	// Set up signal handler for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt)

	ticker := time.NewTicker(time.Millisecond / 10)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			status, err := t.ModemControlPinState()
			if err != nil {
				log.Printf("Error reading modem status: %v", err)
				continue
			}

			// Check if CTS flag is set
			cts := status.Asserted(gpsio.ModemCTS)

			// Check for transition from flag being on to off.
			// This is the opposite of what you might expect.
			// In RS232, CTS is asserted by having a negative voltage, and deasserted by a positive voltage.
			// With a USB to TTL serial adapter, an RS232 negative voltage is represented by a logic low (0V),
			// and a positive voltage is represented by a logic high (3.3V).
			// Thus in TTL, the CTS being asserted corresponds to a logic low (0V);
			// logic high (3.3V) means CTS is deasserted.
			// A PPS leading edge with normal polarity is TTL logic level going from low to high,
			// which correspond to CTS flag going from on to off.
			if !cts && lastCTS {
				now := time.Now()
				ppsCount++
				fmt.Printf("PPS #%d at %s (%d.%09d)\n",
					ppsCount,
					now.Format("15:04:05.000000000"),
					now.Unix(),
					now.Nanosecond())
			}

			lastCTS = cts

		case <-sigChan:
			fmt.Printf("\nReceived interrupt, shutting down...\n")
			return
		}
	}
}

// timeIoctl measures the modem status read directly: back-to-back calls with
// no sleeps, so the distribution of call durations is the poll pacing floor.
// A goroutine drains incoming serial data throughout, because that is the
// condition the daemon polls under: measured on an FT232R on Linux
// ftdi_sio, the read is ~110 us with the port undrained but the daemon,
// which drains it, sees ~1.3 ms brackets on the same adapter.
func timeIoctl(t *gpsio.SerialConn) {
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
	defer func() {
		t.Stop()
		<-done
	}()
	const n = 2000
	ds := make([]time.Duration, n)
	for i := range ds {
		start := time.Now()
		if _, err := t.ModemControlPinState(); err != nil {
			log.Fatalf("Error reading modem status: %v", err)
		}
		ds[i] = time.Since(start)
	}
	slices.Sort(ds)
	var sum time.Duration
	for _, d := range ds {
		sum += d
	}
	fmt.Printf("modem status read over %d calls: min=%v median=%v mean=%v p90=%v max=%v\n",
		n, ds[0], ds[n/2], sum/n, ds[n*9/10], ds[n-1])
}

func usage(progName string, flags *pflag.FlagSet) {
	fmt.Fprintln(os.Stderr, "Usage:", progName, "[-i|--ioctltime] [-s|--speed <baud>] <device>")
	fmt.Fprintln(os.Stderr, "Options:")
	flags.PrintDefaults()
}
