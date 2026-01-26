package cmd

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"

	"golang.org/x/sys/unix"
)

// Exit codes based on BSD sysexits.h
const (
	ExitSuccess = 0
	ExitFailure = 1
	ExitUsage   = 64 // EX_USAGE: command line usage error
	ExitNoPerm  = 77 // EX_NOPERM: permission denied
	ExitConfig  = 78 // EX_CONFIG: configuration error
)

// ConfigError wraps a configuration error discovered after initial config loading.
type ConfigError struct {
	Err error
}

func (e *ConfigError) Error() string { return e.Err.Error() }
func (e *ConfigError) Unwrap() error { return e.Err }

// ConfigErrorf creates a ConfigError with a formatted message.
func ConfigErrorf(format string, args ...any) *ConfigError {
	return &ConfigError{Err: fmt.Errorf(format, args...)}
}

// ExitCode returns the appropriate exit code for an error.
func ExitCode(err error) int {
	if err == nil {
		return ExitSuccess
	}
	var cfgErr *ConfigError
	if errors.As(err, &cfgErr) {
		return ExitConfig
	}
	if errors.Is(err, os.ErrPermission) {
		return ExitNoPerm
	}
	return ExitFailure
}

// ExitCoder interface for errors that specify their own exit code
type ExitCoder interface {
	error
	ExitCode() int
}

func ErrPrintln(progName string, arg any) {
	fmt.Fprintln(os.Stderr, progName+":", arg)
}

// ErrPrintlnWithDetail prints an error and any additional detail from fmt.Stringer.
// Some libraries (like go-toml) provide more detail via String() than Error().
func ErrPrintlnWithDetail(progName string, err error) {
	ErrPrintln(progName, err)
	if s, ok := err.(fmt.Stringer); ok {
		detail := s.String()
		if detail != "" && detail != err.Error() {
			fmt.Fprintln(os.Stderr, detail)
		}
	}
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
