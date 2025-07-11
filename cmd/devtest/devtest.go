//go:build linux

package main

import (
	"fmt"
	"os"
	"time"

	"github.com/jclark/satpulse/internal/devnotify"
)

var runTime = 600 * time.Second

func main() {
	err := run()
	if err != nil {
		fmt.Fprintln(os.Stderr, os.Args[0]+":", err)
		os.Exit(1)
	}
}

func run() error {
	n, err := devnotify.Open()
	if err != nil {
		return err
	}
	timeout := time.After(runTime)
	for {
		select {
		case ev, ok := <-n.Events:
			if !ok {
				return nil
			}
			if ev.Err != nil {
				fmt.Printf("error: %v\n", ev.Err)
			} else if ev.Path != "" {
				fmt.Printf("device path: %s\n", ev.Path)
			} else if ev.IfName != "" {
				fmt.Printf("interface %d: %s\n", ev.IfIndex, ev.IfName)
			}
		case <-timeout:
			fmt.Println("timeout: closing")
			n.Close()
		}
	}

}
