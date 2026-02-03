package daemon

import (
	"errors"
	"fmt"
	"os"
)

// Exit codes based on BSD sysexits.h
const (
	exitSuccess = 0
	exitFailure = 1
	exitUsage   = 64 // EX_USAGE: command line usage error
	exitNoPerm  = 77 // EX_NOPERM: permission denied
	exitConfig  = 78 // EX_CONFIG: configuration error
)

type configError struct {
	err error
}

func (e *configError) Error() string { return e.err.Error() }
func (e *configError) Unwrap() error { return e.err }

func configErrorf(format string, args ...any) *configError {
	return &configError{err: fmt.Errorf(format, args...)}
}

func exitCode(err error) int {
	if err == nil {
		return exitSuccess
	}
	var cfgErr *configError
	if errors.As(err, &cfgErr) {
		return exitConfig
	}
	if errors.Is(err, os.ErrPermission) {
		return exitNoPerm
	}
	return exitFailure
}
