//go:build ignore

// Run from the repository root with:
//
//	go run ./gps/lib/serialenum/listports.go
package main

import (
	"fmt"
	"os"

	"github.com/jclark/satpulse/gps/lib/serialenum"
)

func main() {
	ports, err := serialenum.List()
	if err != nil {
		fmt.Fprintf(os.Stderr, "list serial ports: %v\n", err)
		os.Exit(1)
	}
	for _, port := range ports {
		fmt.Printf("device=%s", port.Device)
		if port.USB != (serialenum.USBID{}) {
			fmt.Printf(" vid=%04x pid=%04x", port.USB.VID, port.USB.PID)
		}
		fmt.Printf(" display=%q\n", port.Display)
	}
}
