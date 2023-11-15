package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"sync"

	"github.com/spf13/pflag"
	"golang.org/x/sys/unix"
)

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


type IntFlag struct {
	Value *int
}

var _ pflag.Value = (*IntFlag)(nil)

func (i *IntFlag) String() string {
	if i.Value == nil {
		return ""
	}
	return strconv.Itoa(*i.Value)
}

func (i *IntFlag) Type() string {
	return "int"
}

func (i *IntFlag) Set(s string) error {
	v, err := strconv.Atoi(s)
	if err != nil {
		return err
	}
	i.Value = &v
	return nil
}
