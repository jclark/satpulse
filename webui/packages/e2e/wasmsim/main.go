// wasmsim runs the receiver simulator on stdin/stdout for browser tests.
package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/jclark/satpulse/gps/app/ubxsim"
	ucv "github.com/jclark/satpulse/gps/lib/ubxcfgval"
)

type stdio struct{}

func main() {
	p, err := ubxsim.LoadPersonality(os.Args[1])
	if err != nil {
		panic(err)
	}
	if err := p.LoadReplay(os.Args[2]); err != nil {
		panic(err)
	}
	s := ubxsim.New(p, ubxsim.Options{Port: ucv.UART1, Logger: slog.New(slog.DiscardHandler)})
	if err := s.Run(context.Background(), stdio{}); err != nil {
		panic(err)
	}
}

// Read receives receiver commands from the browser test harness.
func (stdio) Read(b []byte) (int, error) { return os.Stdin.Read(b) }

// Write returns receiver packets to the browser test harness.
func (stdio) Write(b []byte) (int, error) { return os.Stdout.Write(b) }

// Close stops the simulator's command input.
func (stdio) Close() error { return os.Stdin.Close() }
