//go:build none

// Tiny driver for ad-hoc testing: go run -tags none ./time/lib/sntp <server>
package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/jclark/satpulse/time/lib/sntp"
)

func main() {
	timeout := flag.Duration("timeout", 5*time.Second, "overall timeout including DNS")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: %s [-timeout d] <ntp-server>\n", os.Args[0])
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(2)
	}
	r, err := sntp.Query(flag.Arg(0), *timeout)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
	now := time.Now()
	fmt.Printf("server time   : %s\n", r.Time.Format(time.RFC3339Nano))
	fmt.Printf("system time   : %s\n", now.UTC().Format(time.RFC3339Nano))
	fmt.Printf("system offset : %s\n", r.Time.Sub(now))
	fmt.Printf("accuracy      : %s\n", r.Accuracy)
	fmt.Printf("leap          : %d\n", r.Leap)
}
