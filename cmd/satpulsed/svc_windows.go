//go:build windows

package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/jclark/satpulse/gps/app/cmd"
	"github.com/jclark/satpulse/time/app/daemon"
	"github.com/spf13/pflag"
	"golang.org/x/sys/windows/registry"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/eventlog"
	"golang.org/x/sys/windows/svc/mgr"
)

const serviceName = "satpulsed"

// Event Log event IDs for the service-lifecycle events written by the
// service wrapper. Everything the daemon itself logs goes to --log-file.
const (
	evStarting = iota + 1
	evRunning
	evStopRequested
	evStopped
	evStartFailed
	evExitError
)

// Registry values under the service's Parameters key recording what
// --install created, so --uninstall removes only what install owns.
const (
	regParamsPath = `SYSTEM\CurrentControlSet\Services\` + serviceName + `\Parameters`
	regCopiedExe  = "CopiedExe"
	regCreatedDir = "CreatedDir"
)

type platformVars struct {
	install    bool
	uninstall  bool
	winSvc     bool
	logFile    string
	installDir string
}

const platformSummary = `
       [--install [--install-dir dir]|--uninstall|--win-svc] [--log-file path]`

func registerPlatformFlags(flags *pflag.FlagSet, vars *flagVars) {
	p := &vars.platform
	flags.BoolVar(&p.install, "install", false, "register satpulsed as a Windows service")
	flags.BoolVar(&p.uninstall, "uninstall", false, "remove the satpulsed Windows service")
	flags.BoolVar(&p.winSvc, "win-svc", false, "run as a Windows service (for use by the Service Control Manager only)")
	flags.StringVar(&p.logFile, "log-file", "", "file the daemon log is written to in service mode")
	flags.StringVar(&p.installDir, "install-dir", "", "directory the executable is copied to by --install")
}

// platformMain dispatches the Windows service run modes; if none is
// selected it returns and the caller runs the foreground path.
// Uninstall comes first and needs no config: the config file may be
// missing or broken by the time the service is removed.
func platformMain(progName string, vars *flagVars) {
	p := &vars.platform
	var err error
	if p.uninstall {
		err = uninstallService()
	} else if p.install {
		err = installService(vars)
	} else if p.winSvc {
		err = runService(vars)
	} else {
		return
	}
	if err != nil {
		cmd.ErrPrintln(progName, err)
		os.Exit(1)
	}
	os.Exit(0)
}

// runService runs the daemon under the Service Control Manager.
func runService(vars *flagVars) error {
	elog, err := eventlog.Open(serviceName)
	if err != nil {
		return err
	}
	defer elog.Close()
	if err := svc.Run(serviceName, &service{vars: vars, elog: elog}); err != nil {
		return fmt.Errorf("cannot connect to the service control manager (--win-svc is for use by the SCM only; use --install and then 'sc start %s'): %w", serviceName, err)
	}
	return nil
}

type service struct {
	vars *flagVars
	elog *eventlog.Log
}

// Execute is the svc.Handler; it maps SCM Stop/Shutdown requests onto the
// daemon's context cancellation. Failures are reported with a
// service-specific exit code: with svcSpecificEC false the SCM would
// misread the daemon's sysexits-style codes as Win32 error numbers. The
// real detail is in the Event Log and the daemon log file.
func (s *service) Execute(_ []string, r <-chan svc.ChangeRequest, status chan<- svc.Status) (svcSpecificEC bool, exitCode uint32) {
	status <- svc.Status{State: svc.StartPending}
	s.elog.Info(evStarting, serviceName+" starting")
	cfg, lg, closeLog, err := s.startup()
	if err != nil {
		s.elog.Error(evStartFailed, serviceName+" failed to start: "+err.Error())
		status <- svc.Status{State: svc.Stopped}
		return true, 1
	}
	defer closeLog()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- daemon.Run(ctx, cfg, lg) }()
	status <- svc.Status{State: svc.Running, Accepts: svc.AcceptStop | svc.AcceptShutdown}
	s.elog.Info(evRunning, serviceName+" running")
	for {
		select {
		case req := <-r:
			switch req.Cmd {
			case svc.Interrogate:
				status <- req.CurrentStatus
			case svc.Stop, svc.Shutdown:
				status <- svc.Status{State: svc.StopPending}
				s.elog.Info(evStopRequested, serviceName+" stop requested")
				cancel()
			}
		case err := <-done:
			status <- svc.Status{State: svc.Stopped}
			if err != nil {
				s.elog.Error(evExitError, serviceName+" exited with error: "+err.Error())
				return true, 1
			}
			s.elog.Info(evStopped, serviceName+" stopped")
			return false, 0
		}
	}
}

// startup loads the config and opens the log file. It runs inside Execute
// so that a failure is reported to the SCM as StartPending -> Stopped with
// an Event Log entry, not a silent exit before the service handler runs.
func (s *service) startup() (*daemon.Config, *slog.Logger, func(), error) {
	if s.vars.platform.logFile == "" {
		return nil, nil, nil, fmt.Errorf("service mode requires --log-file")
	}
	if err := requireConfigFiles(s.vars); err != nil {
		return nil, nil, nil, err
	}
	cfg, configPath, err := daemon.LoadConfig(s.vars.configFiles...)
	if err != nil {
		return nil, nil, nil, err
	}
	applyOverrides(cfg, s.vars)
	f, err := os.OpenFile(s.vars.platform.logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, nil, nil, err
	}
	lg := slog.New(slog.NewTextHandler(f, &slog.HandlerOptions{Level: logLevel(cfg, s.vars)}))
	slog.SetDefault(lg)
	ver, buildDate := cmd.Version()
	lg.Info("starting", "version", ver, "buildDate", buildDate, "configPath", configPath)
	return cfg, lg, func() { f.Close() }, nil
}

// installService registers satpulsed with the SCM. The registered command
// carries the config and log-file paths explicitly, resolved to absolute
// paths, because the SCM launches services with a different working
// directory. With --install-dir the executable is first copied there, so
// the running service does not lock the development binary.
func installService(vars *flagVars) error {
	p := &vars.platform
	if len(vars.configFiles) == 0 {
		return fmt.Errorf("--install requires a config file specified with -f")
	}
	if p.logFile == "" {
		return fmt.Errorf("--install requires --log-file")
	}
	logPath, err := filepath.Abs(p.logFile)
	if err != nil {
		return err
	}
	var configPaths []string
	for _, f := range vars.configFiles {
		a, err := filepath.Abs(f)
		if err != nil {
			return err
		}
		if _, err := os.Stat(a); err != nil {
			return err
		}
		configPaths = append(configPaths, a)
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	copiedExe, createdDir := "", ""
	if p.installDir != "" {
		dir, err := filepath.Abs(p.installDir)
		if err != nil {
			return err
		}
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			if err := os.MkdirAll(dir, 0755); err != nil {
				return err
			}
			createdDir = dir
		} else if err != nil {
			return err
		}
		copiedExe = filepath.Join(dir, "satpulsed.exe")
		if err := copyFile(exe, copiedExe); err != nil {
			return err
		}
		exe = copiedExe
	}
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer m.Disconnect()
	if s, err := m.OpenService(serviceName); err == nil {
		s.Close()
		return fmt.Errorf("service %s already exists", serviceName)
	}
	args := []string{"--win-svc", "--log-file", logPath}
	for _, f := range configPaths {
		args = append(args, "-f", f)
	}
	s, err := m.CreateService(serviceName, exe, mgr.Config{
		StartType:   mgr.StartAutomatic,
		DisplayName: "SatPulse Daemon",
		Description: "GPS receiver monitoring and time synchronization daemon",
	}, args...)
	if err != nil {
		return err
	}
	defer s.Close()
	// Remove any leftover source from an earlier install so registration
	// is idempotent.
	_ = eventlog.Remove(serviceName)
	if err := eventlog.InstallAsEventCreate(serviceName, eventlog.Error|eventlog.Warning|eventlog.Info); err != nil {
		return fmt.Errorf("registering the event log source: %w", err)
	}
	if copiedExe != "" {
		k, _, err := registry.CreateKey(registry.LOCAL_MACHINE, regParamsPath, registry.SET_VALUE)
		if err != nil {
			return err
		}
		defer k.Close()
		if err := k.SetStringValue(regCopiedExe, copiedExe); err != nil {
			return err
		}
		if createdDir != "" {
			if err := k.SetStringValue(regCreatedDir, createdDir); err != nil {
				return err
			}
		}
	}
	fmt.Printf("installed service %s: %s %s\n", serviceName, exe, argsString(args))
	return nil
}

func argsString(args []string) string {
	s := ""
	for i, a := range args {
		if i > 0 {
			s += " "
		}
		s += a
	}
	return s
}

func copyFile(src, dst string) error {
	b, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, b, 0755)
}

// uninstallService removes the service, its Event Log source, and any
// executable copy (and directory) that install created. It needs no
// config file, so it works even when the config is gone or broken.
func uninstallService() error {
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer m.Disconnect()
	s, err := m.OpenService(serviceName)
	if err != nil {
		return fmt.Errorf("service %s is not installed", serviceName)
	}
	defer s.Close()
	copiedExe, createdDir := readInstallParams()
	if err := stopService(s); err != nil {
		return err
	}
	if err := s.Delete(); err != nil {
		return err
	}
	_ = eventlog.Remove(serviceName)
	if copiedExe != "" {
		if err := os.Remove(copiedExe); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	if createdDir != "" {
		// Remove fails on a non-empty directory, which is what we want: a
		// config or log the user has put there survives.
		_ = os.Remove(createdDir)
	}
	fmt.Printf("removed service %s\n", serviceName)
	return nil
}

func readInstallParams() (copiedExe, createdDir string) {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, regParamsPath, registry.QUERY_VALUE)
	if err != nil {
		return "", ""
	}
	defer k.Close()
	copiedExe, _, _ = k.GetStringValue(regCopiedExe)
	createdDir, _, _ = k.GetStringValue(regCreatedDir)
	return copiedExe, createdDir
}

// stopService stops a running service and waits for it to reach Stopped,
// because a running service locks its executable.
func stopService(s *mgr.Service) error {
	st, err := s.Query()
	if err != nil {
		return err
	}
	if st.State == svc.Stopped {
		return nil
	}
	if st.State != svc.StopPending {
		if st, err = s.Control(svc.Stop); err != nil {
			return err
		}
	}
	deadline := time.Now().Add(30 * time.Second)
	for st.State != svc.Stopped {
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out waiting for service %s to stop", serviceName)
		}
		time.Sleep(300 * time.Millisecond)
		if st, err = s.Query(); err != nil {
			return err
		}
	}
	return nil
}
