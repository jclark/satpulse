package main

import (
	"flag"
	"fmt"
	"net"
	"os"
	"time"

	"github.com/jclark/gps4ptp/netnotify"
)

var ifName string
var runTime = 30 * time.Second

func main() {
	flag.StringVar(&ifName, "e", "eth0", "ethernet interface to monitor")
	flag.Parse()
	err := run()
	if err != nil {
		fmt.Fprintln(os.Stderr, os.Args[0]+":", err)
		os.Exit(1)
	}
}

func run() error {
	iface, err := net.InterfaceByName(ifName)
	if err != nil {
		return err
	}
	fmt.Printf("initial flags: %s\n", iface.Flags.String())
	n, err := netnotify.OpenNotifier()
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
			}
			if ev.Index == iface.Index {
				fmt.Printf("flags: %s\n", ev.Flags.String())
			}
		case <-timeout:
			fmt.Println("timeout: closing")
			n.Close()
		}
	}
}
