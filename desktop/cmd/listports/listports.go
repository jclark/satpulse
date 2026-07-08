package main

import (
	"fmt"
	"os"

	"github.com/jclark/satpulse/desktop/serialenum"
)

func main() {
	ports, err := serialenum.List()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	for _, p := range ports {
		fmt.Printf("%s %s\n", p.Device, p.Display)
	}
}
