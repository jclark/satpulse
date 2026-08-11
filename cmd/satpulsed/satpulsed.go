package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/jclark/satpulse/gps/app/cmd"
	"github.com/jclark/satpulse/time/app/daemon"
)

func main() {
	progName := os.Args[0]
	vars, msg, err := parseFlags(progName, os.Args[1:])
	if vars == nil {
		exitCode := 0
		if err != nil {
			cmd.ErrPrintln(progName, err)
			exitCode = daemon.ExitUsage
		}
		fmt.Fprint(os.Stderr, msg)
		os.Exit(exitCode)
	}
	// On Windows this may run a service mode (register, unregister, or the
	// SCM-driven service itself) and exit instead of returning.
	platformMain(progName, vars)
	runForeground(progName, vars)
}

func runForeground(progName string, vars *flagVars) {
	if err := requireConfigFiles(vars); err != nil {
		cmd.ErrPrintln(progName, err)
		os.Exit(daemon.ExitUsage)
	}
	cfg, configPath, err := daemon.LoadConfig(vars.configFiles...)
	if err != nil {
		cmd.ErrPrintlnWithDetail(progName, err)
		os.Exit(daemon.ExitConfig)
	}
	applyOverrides(cfg, vars)
	var handler slog.Handler
	if vars.sdLog {
		handler = daemon.NewSdHandler(logLevel(cfg, vars), os.Stdout)
	} else {
		handler = slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: logLevel(cfg, vars)})
	}
	lg := slog.New(handler)
	slog.SetDefault(lg)
	ver, buildDate := cmd.Version()
	lg.Info("starting", "version", ver, "buildDate", buildDate, "configPath", configPath)
	ctx, _ := cmd.CancelOnSignal(context.Background(), lg)
	if err := daemon.Run(ctx, cfg, lg); err != nil {
		cmd.ErrPrintln(progName, err)
		os.Exit(daemon.ExitCode(err))
	}
}

func applyOverrides(cfg *daemon.Config, vars *flagVars) {
	if vars.wait {
		cfg.PHC.Wait = true
	}
	if vars.serialDevice != "" {
		cfg.Serial.Device = vars.serialDevice
	}
}

func logLevel(cfg *daemon.Config, vars *flagVars) slog.Level {
	if vars.verbose || cfg.Log.Verbose {
		return slog.LevelDebug
	}
	return slog.LevelInfo
}
