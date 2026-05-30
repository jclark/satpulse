package main

import (
	"io"
	"log/slog"

	"github.com/jclark/satpulse/internal/annotatecmd"
	"github.com/jclark/satpulse/internal/convobscmd"
	"github.com/jclark/satpulse/internal/decodecmd"
	"github.com/jclark/satpulse/internal/gpscmd"
	"github.com/jclark/satpulse/internal/ntripcmd"
	"github.com/jclark/satpulse/internal/packcmd"
	"github.com/jclark/satpulse/internal/replaycmd"
)

type cmdFunc func(logWriter io.Writer, logLevel slog.Level, progName string, cmdName string, cmdArgs []string) (usage string, err error)

// commands maps subcommand names to their entry points. Platform-specific
// subcommands are added via init functions in build-tagged files.
var commands = map[string]cmdFunc{
	"annotate": annotatecmd.Cmd,
	"convobs":  convobscmd.Cmd,
	"decode":   decodecmd.Cmd,
	"gps":      gpscmd.Cmd,
	"ntrip":    ntripcmd.Cmd,
	"pack":     packcmd.Cmd,
	"replay":   replaycmd.Cmd,
}
