//go:build !windows

// This is an experiment for Darwin to try polling the CTS line to detect PPS signals.
// The GPS PPS output should be connected to the CTS pin of a USB to TTL adapter.
// I have tested this with the Waveshare USB to TTL converter, which uses the FTDI FT232RNL.
package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"time"

	"github.com/jclark/satpulse/gps/lib/term"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintf(os.Stderr, "usage: %s <device>\n", os.Args[0])
		os.Exit(1)
	}

	device := os.Args[1]

	// Open terminal in raw mode
	t, err := term.Open(device, term.RawMode)
	if err != nil {
		log.Fatalf("Failed to open %s: %v", device, err)
	}
	defer func() {
		t.Restore()
		t.Close()
	}()

	fmt.Printf("Monitoring PPS on CTS line of %s\n", device)

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
			status, err := t.ModemStatus()
			if err != nil {
				log.Printf("Error reading modem status: %v", err)
				continue
			}

			// Check if CTS flag is set
			cts := (status & term.MODEM_CTS) != 0

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
