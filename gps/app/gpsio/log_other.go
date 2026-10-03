//go:build (!unix && !windows) || baremetal

package gpsio

import "os"

// Platforms without signals never reopen the log file.
func notifyReopen(chan<- os.Signal) {}
