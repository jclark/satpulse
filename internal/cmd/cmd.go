package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"sync"

	"golang.org/x/sys/unix"
)

// ExitCoder interface for errors that specify their own exit code
type ExitCoder interface {
	error
	ExitCode() int
}

func ErrPrintln(progName string, arg any) {
	fmt.Fprintln(os.Stderr, progName+":", arg)
}

func WaitGroupGo(wg *sync.WaitGroup, f func()) {
	wg.Add(1)
	go func() {
		defer wg.Done()
		f()
	}()
}

func CancelOnSignal(ctx context.Context, lg *slog.Logger) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(ctx)
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, unix.SIGTERM)
	go func() {
		<-sig
		lg.Debug("received signal, initiating cancellation")
		cancel()
	}()
	return ctx, cancel
}
