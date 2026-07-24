package cmd

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/jclark/satpulse/gps/gpsreg"
)

// ResolveVendors returns the vendor list in effect: the explicitly
// specified vendor if any, else the SATPULSE_VENDORS declaration, else
// nil. A non-nil error (a malformed declaration) is fatal at startup.
func ResolveVendors(vendor gpsreg.Vendor) ([]gpsreg.Vendor, error) {
	if vendor != 0 {
		return []gpsreg.Vendor{vendor}, nil
	}
	return gpsreg.EnvVendors()
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
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sig
		lg.Debug("received signal, initiating cancellation")
		cancel()
	}()
	return ctx, cancel
}

// NewLogger creates a standard text logger with the given writer and level.
func NewLogger(w io.Writer, level slog.Level) *slog.Logger {
	return slog.New(slog.NewTextHandler(w, &slog.HandlerOptions{Level: level}))
}

// NewDefaultLogger creates a standard text logger with the given writer and level,
// and sets it as the process-wide default logger.
func NewDefaultLogger(w io.Writer, level slog.Level) *slog.Logger {
	lg := NewLogger(w, level)
	slog.SetDefault(lg)
	return lg
}
