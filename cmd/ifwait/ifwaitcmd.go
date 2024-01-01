package main

import (
	"fmt"
	"net"
	"os"
	"time"

	"github.com/jclark/satpulse/internal/cmd"
	"github.com/jclark/satpulse/internal/ifwait"
	"github.com/spf13/pflag"
)

func main() {
	var help bool
	var showVersion bool
	var carrier bool
	var up bool
	var timeout time.Duration

	flags := pflag.NewFlagSet("ifwait", pflag.ContinueOnError)

	flags.BoolVarP(&help, "help", "h", false, "show help")
	flags.BoolVarP(&showVersion, "version", "V", false, "show version information")
	flags.BoolVarP(&carrier, "carrier", "c", false, "wait for interface to have a carrier")
	flags.BoolVarP(&up, "up", "u", false, "wait for interface to be up")
	flags.DurationVarP(&timeout, "timeout", "t", 0, "length of time to wait before timing out (e.g. 10s, 1m)")

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
	iflags := net.Flags(0)
	if up {
		iflags |= net.FlagUp
	}
	if carrier {
		iflags |= net.FlagRunning
	}
	now := time.Now()
	ifname := flags.Args()[0]
	err = run(ifname, iflags, timeout)
	if err != nil {
		cmd.ErrPrintln(progName, err)
		os.Exit(1)
	}
	waited := time.Since(now)
	fmt.Fprintf(os.Stderr, "%s: interface %s: waited for %s\n", progName, ifname, waited.String())
}

func run(ifname string, wantedFlags net.Flags, timeout time.Duration) error {
	watcher, err := ifwait.NewIfWaiter(ifname)
	if err != nil {
		return err
	}
	defer watcher.Close()
	ch := watcher.SetCond(func(flags net.Flags) bool {
		return flags&wantedFlags == wantedFlags
	})
	var tCh <-chan time.Time
	if timeout != 0 {
		tCh = time.After(timeout)
	}
	select {
	case _, ok := <-ch:
		if !ok {
			return watcher.Err()
		}
	case <-tCh:
		return fmt.Errorf("interface %s: timeout", ifname)
	}
	return nil
}

func usage(progName string, flags *pflag.FlagSet) {
	fmt.Fprintln(os.Stderr, "Usage:", progName, "[-u|--up] [-c|--carrier] [-t|--timeout duration] <interface-name>")
	fmt.Fprintln(os.Stderr, "Options:")
	flags.PrintDefaults()
}
