//go:build ignore

package main

import (
	"fmt"
	"os"
	"time"

	"github.com/jclark/satpulse/time/lib/fuser"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintf(os.Stderr, "Usage: %s <path>\n", os.Args[0])
		os.Exit(1)
	}

	targetPath := os.Args[1]

	start := time.Now()
	pids, err := fuser.ListPIDs(targetPath)
	t := time.Since(start)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	for _, pid := range pids {
		fmt.Println(pid)
	}

	fmt.Fprintf(os.Stderr, "Scan took %v\n", t)
}
