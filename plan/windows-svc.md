# Windows service support for satpulsed (#338)

The Windows compile port is done: the whole tree, including `satpulsed`,
cross-compiles and runs from a console on Windows (serial support, the
config-panic fix, and the CI build workflow all landed). This plan adds
service integration on top of that base.

## Goal

Run `satpulsed` as a Windows service: an SCM-managed background process
that starts at boot, shuts down cleanly on a service stop, and reuses the
existing OS-independent daemon code. PHC/PTP time synchronisation is
Linux-only and stays disabled on Windows (the existing `clk == nil`
path, as on macOS), but the rest of the daemon works: GPS receiver
configuration, the Ntrip caster, stream pull/push, the web dashboard and
SSE, the `/position` endpoint, Prometheus metrics, and packet/event/track
logging.

## Background and current state

- `gps/lib/term` already has a full Win32 serial implementation
  (`term_windows.go`), so receiver I/O over a `COM` port works.
- `daemon.run` already runs without a PHC (`clk == nil`); every PHC/PTP
  component is behind that check, so the macOS-style feature set is what
  Windows gets.
- `satpulsed.exe` already builds and runs in the foreground on Windows
  (stdout logging, signal-based shutdown). What remains is SCM
  integration: a service run mode, install/uninstall, a file/Event Log
  logging path, and the daemon refactor that lets the SCM drive shutdown.

## Step 1: refactor daemon to a context + config + logger entry point

Today `daemon.Cmd` owns flag parsing, config loading, logger
construction, signal handling, and `os.Exit`. Move the OS-specific and
CLI-specific parts up into `cmd/satpulsed` so the daemon becomes a pure,
portable run function. The service path needs a different logger sink (no
console) and a different cancellation source (the SCM control channel,
not signals), which is exactly what the caller should own.

- New daemon entry point:

  ```go
  // Run executes the daemon until ctx is cancelled or it exits on its
  // own, and returns the settled error. The caller owns signal/service
  // handling, logger construction, and process exit.
  //
  // A plain cancellation of ctx (the normal stop path: a signal, or an
  // SCM Stop) is success and returns nil. Run returns a non-nil error
  // only for a genuine fault -- a startup failure, or a cancellation
  // cause other than context.Canceled, such as the scan worker reporting
  // that serial input disappeared.
  func Run(ctx context.Context, cfg *Config, lg *slog.Logger) error
  ```

  `Run` creates its own `context.WithCancelCause` child internally and
  preserves today's logic exactly (`daemon.go:81`): it promotes the
  cancellation cause to the returned error only when that cause is
  non-nil and not `context.Canceled`. This contract matters for the
  service: an `sc stop satpulsed` must return nil so it logs a clean
  "stopped" Event Log entry and a zero exit, not a spurious error.
- Keep `LoadConfig`, `Config.Validate`, and the exit-code mapping in the
  daemon package (they are OS-independent); export the mapping as
  `ExitCode(err) int`.
- Move `flags.go` (the `pflag` set and `flagVars`) into `cmd/satpulsed`.
- This is a behaviour-preserving refactor for the Unix/foreground path.

## Step 2: cmd/satpulsed dispatch

The non-Windows main keeps today's behaviour: parse flags, `LoadConfig`,
apply the `--wait` / `--serial-device` / verbosity overrides, build a
stdout logger, build a signal-cancelled context via `cmd.CancelOnSignal`,
call `daemon.Run`, then `os.Exit(daemon.ExitCode(err))`.

The service flags and their handling live in a Windows-tagged file
(`svc_windows.go`, `//go:build windows`); a `//go:build !windows`
counterpart provides no-op registration and dispatch so the tree builds
everywhere.

- `registerPlatformFlags(*pflag.FlagSet, *flagVars)` adds `--install`,
  `--uninstall`, `--win-svc`, `--log-file`, and `--install-dir` on Windows
  only, so they never appear in the Unix help text. `--log-file` sets
  where the daemon's slog output is written in service mode (see step 3);
  `--install-dir` is used with `--install` (see step 4).
- The mandatory-config check (a config path via `-f` or the env var)
  currently lives inside the parser (`flags.go:54`). Moving parsing up
  lets each mode decide for itself, which matters because **`--uninstall`
  must work with no config at all** -- the config file may have been
  deleted or broken by the time the service is removed.
- Dispatch order on Windows, immediately after flag parsing and **before**
  any config is required or loaded:
  1. `--uninstall` -> unregister the service and clean up (step 4), then
     exit. Requires neither `-f` nor a readable config.
  2. `--install` -> requires `-f` and `--log-file`; register the service
     (step 4), then exit. It may stat the config path to fail fast, but
     does not need a fully valid config.
  3. `--win-svc` -> open the Event Log source, then call `svc.Run`
     straight away. Config loading and file-logger construction happen
     **inside** the `Execute` handler, not before it, so any startup
     failure (bad config, unwritable `--log-file`) is reported to the SCM
     as `StartPending -> Stopped` with an Event Log error entry, instead
     of a silent pre-service exit the SCM only sees as a generic timeout.
  4. otherwise -> foreground, identical to the Unix path (config
     required as today).

`--win-svc` is not auto-detected; it is explicit. `--install` bakes it
into the registered command (see step 4), so the SCM always launches with
it, and there is no need for `svc.IsWindowsService()` heuristics. If a
user runs `satpulsed --win-svc` by hand, `svc.Run` fails to connect to
the service dispatcher; map that error to a message noting that
`--win-svc` is for the Service Control Manager only.

### The svc.Run handler

`Execute` maps SCM control requests onto the daemon's existing
context-cancellation shutdown:

```go
func (s *service) Execute(_ []string, r <-chan svc.ChangeRequest, status chan<- svc.Status) (svcSpecificEC bool, exitCode uint32) {
    status <- svc.Status{State: svc.StartPending}
    s.elog.Info(evStarting, serviceName)

    // Load config and open the file logger here, inside Execute, so a
    // startup failure is reported to the SCM and the Event Log rather
    // than exiting before the service handler runs.
    cfg, lg, err := s.startup() // LoadConfig + open --log-file slog handler
    if err != nil {
        s.elog.Error(evStartFailed, err.Error())
        status <- svc.Status{State: svc.Stopped}
        return true, 1 // service-specific: SCM won't misread it as Win32
    }

    ctx, cancel := context.WithCancel(context.Background())
    done := make(chan error, 1)
    go func() { done <- daemon.Run(ctx, cfg, lg) }()
    status <- svc.Status{State: svc.Running, Accepts: svc.AcceptStop | svc.AcceptShutdown}
    s.elog.Info(evRunning, serviceName)
    for {
        select {
        case req := <-r:
            switch req.Cmd {
            case svc.Interrogate:
                status <- req.CurrentStatus
            case svc.Stop, svc.Shutdown:
                status <- svc.Status{State: svc.StopPending}
                s.elog.Info(evStopRequested, serviceName)
                cancel()
            }
        case err := <-done:
            status <- svc.Status{State: svc.Stopped}
            if err != nil {
                s.elog.Error(evExitError, err.Error())
                return true, 1 // service-specific error code
            }
            s.elog.Info(evStopped, serviceName)
            return false, 0 // clean stop
        }
    }
}
```

The SCM Stop/Shutdown request pulls the same `cancel` lever that a signal
pulls on Unix. The `elog` entries are the lifecycle/terminal-error events
of step 3; `s.elog` is the `*eventlog.Log` opened by the wrapper before
`svc.Run`, and the slog file logger carries everything else.

Two deliberate choices here. First, `Execute` returns `svcSpecificEC =
true` with a small service-specific code on failure: with the flag
`false`, the SCM interprets the `uint32` as a Win32 error, so a daemon
exit code like 78 (`EX_CONFIG`) would be mis-rendered as a system error.
Mapping any failure to a single service-specific code keeps the SCM's
"the service terminated with error" message accurate; the real detail is
already in the Event Log and the file. Second, config/logger startup is
inside `Execute` (not in the dispatch before `svc.Run`) precisely so the
`StartPending -> Stopped` transition and the `evStartFailed` event fire
on a bad config -- otherwise the SCM only sees a process that never
reported Running.

## Step 3: logging in service mode

Two distinct sinks, with a clean division of responsibility:

- The daemon's `slog.Logger` -- the full structured log stream, including
  every error the daemon logs while running -- writes to a **file** in
  service mode (there is no console under the SCM). The path comes from
  the Windows-only `--log-file` option. The logger is constructed by
  `cmd/satpulsed` and passed to `daemon.Run`, so this is entirely a
  command-line-layer concern and the daemon is unchanged.
- The Windows **Event Log** records the service-lifecycle events visible
  at the `cmd/satpulsed` level -- as much as the wrapper itself can see,
  no more. That is the SCM state transitions it drives (service starting,
  running, stop/shutdown requested, stopped) plus the **terminal error**
  returned by `daemon.Run` (and any startup error that prevents `Run`
  from being reached, such as a bad config path or an unwritable log
  file): an error entry on a non-nil error, an informational entry on a
  clean stop. The daemon's internal activity is not visible to the
  wrapper and stays in the file. These entries are written via
  `svc/eventlog` **directly from the `svc_windows.go` wrapper**, never
  from the daemon package, which stays portable and Windows-unaware.

The Event Log is, in effect, the "is the service alive, and why did it
stop or fail to start" signal, in the place a Windows admin looks.
Because it does not depend on the file sink, a failed start is
diagnosable from Event Viewer even when `--log-file` itself is
misconfigured. The event source is registered and removed as part of
install/uninstall (step 4).

## Step 4: install and uninstall

Use `golang.org/x/sys/windows/svc/mgr` (already available via the
existing `golang.org/x/sys` dependency):

- `--install` requires both `-f` and `--log-file`. A service launches
  non-interactively, so the config and log-file locations must be carried
  explicitly in the registered command -- there is no console fallback,
  and relying on a `satpulse.toml` env var is fragile under the SCM.
- All three paths are resolved to **absolute** paths before being baked
  in, because a service runs with a different working directory (the SCM
  default is `C:\Windows\System32`), not wherever `--install` was run.
- By default `--install` registers the running executable in place
  (`os.Executable()`). If `--install-dir <dir>` is given, the executable
  is first copied to `<dir>\satpulsed.exe` and that copy is registered
  instead. This matters on Windows because a running service holds its
  image file open: registering the build-output binary would make the
  next `go install` / rebuild fail trying to overwrite the locked file.
  Copying to a stable location (e.g. `C:\Program Files\SatPulse`) decouples
  the service image from the development binary.
- Install then: `mgr.Connect`, `m.CreateService(name, exe,
  mgr.Config{StartType: mgr.StartAutomatic, ...}, "--win-svc", "-f",
  configPath, "--log-file", logPath)`, where `exe` is the in-place or
  copied executable path. The registered command is therefore
  `C:\...\satpulsed.exe --win-svc -f C:\...\satpulse.toml --log-file
  C:\...\satpulsed.log`. Also register the Event Log source with
  `eventlog.InstallAsEventCreate(name, eventlog.Error|eventlog.Warning|
  eventlog.Info)`.
- So that `--uninstall` can clean up a copied binary -- but never the
  user's in-place development binary -- install records what it owns under
  the service's registry parameters (`...\Services\satpulsed\Parameters`):
  the copied executable path, and whether install created the directory.
  An in-place install records nothing to remove.
- `--uninstall`:
  1. Open the service and read the ownership info recorded at install.
  2. Stop the service if running and wait for `Stopped`, because the
     running executable is locked and cannot be deleted otherwise.
  3. `DeleteService` and `eventlog.Remove(name)`.
  4. If install recorded a copied executable, delete it; if install also
     created the directory, remove the directory too. A pre-existing
     directory (and anything else in it, such as a config the user placed
     there) is left alone.
- Install needs only the config *path*, not a fully valid config: the
  daemon parses it at service start, inside `Execute` (step 2). A cheap
  `stat` of the path catches an obvious typo without duplicating
  validation.
- `start` / `stop` are intentionally omitted; `sc start satpulsed` and
  `net stop satpulsed` already do that.

## Testing

- `make test` / `go test ./...` on Linux and macOS are unchanged: the
  daemon refactor (step 1) is behaviour-preserving on the foreground
  path, and all the new code is Windows-tagged.
- `build-windows.yml` stays green with the daemon refactor and the new
  Windows-tagged files.
- On a Windows host with a receiver on a `COM` port: `satpulsed
  --install -f <config> --log-file <path>`, `sc start satpulsed`, confirm
  the service reaches Running and writes the log file, scrape `/metrics`,
  open the web dashboard, exercise an Ntrip mountpoint, then `sc stop
  satpulsed` and confirm a clean Stopped with a "stopped" (not error)
  Event Log entry and zero exit, then `satpulsed --uninstall` and confirm
  the service, the Event Log source, and any copied binary/directory are
  gone.
- Negative checks: a deliberately bad `-f` under the service reports
  `evStartFailed` in the Event Log and a `Stopped` status (not a hang);
  `--uninstall` succeeds even with the config file deleted; a hand-run
  `satpulsed --win-svc` prints the SCM-only message rather than hanging.

## Release notes

This is a user-facing feature, so completing it requires a
`docs/_includes/NEWS.md` entry under the current unreleased version, in
the Miscellaneous section, referencing the issue number. The website
setup/tutorial docs should also gain a short Windows-service section.

## Files changed or added

(The compile fixes, `win-build.ps1` satpulsed build, and CI changes are
part of the already-completed Windows compile port, not here.)

- `time/app/daemon/daemon.go` -- new `Run(ctx, cfg, lg)`; `Cmd` slimmed
  or removed.
- `time/app/daemon/exit.go` -- export `ExitCode`.
- `time/app/daemon/flags.go` -- moved into `cmd/satpulsed`.
- `cmd/satpulsed/satpulsed.go` -- parsing, logger, foreground dispatch.
- `cmd/satpulsed/svc_windows.go` --
  `--install`/`--uninstall`/`--win-svc`/`--log-file`/`--install-dir`, the
  `svc.Run` handler, `mgr`-based install/uninstall (with optional
  executable copy and absolute-path resolution), and the `eventlog`
  service-lifecycle/terminal-error events.
- `cmd/satpulsed/svc_other.go` -- `//go:build !windows` no-op
  `registerPlatformFlags` and dispatch stub.
- `docs/_includes/NEWS.md` and website setup docs.
