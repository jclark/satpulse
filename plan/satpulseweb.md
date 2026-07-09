# Serve experimental desktop GUI as web app (#357)

A new binary `cmd/satpulsewb` that serves the desktop GUI as a web
app called SatPulse Workbench (matching the workbench npm package the
frontend lives in): an HTTP server with an embedded single-page
frontend, driven by an application core extracted from the desktop
backend into a new package `gps/app/session`. The top priority is device-independent
receiver configuration through a GUI that requires no knowledge of
the receiver's protocol; the full desktop feature set (monitor,
packets, message files, corrections) comes along because it flows
through the same core.

The work is delivered as a stack of individually reviewable PRs, one
per phase, each branching off the previous phase's branch rather than
master, with the Wails desktop app reworked on top (on the
desktop-gui branch, phase 4). The phase structure and the branching and
history-preservation strategy are described under Delivery below.

Prerequisite: the web toolchain reorganisation (#283,
[web-toolchain.md](web-toolchain.md)), which creates the `webui/` npm
workspace the frontend work lands in. Related: #284 (web redesign).

## Motivation

- Many users find the `satpulsetool gps` command line unappealing;
  the device-independent config engine makes a GUI possible that no
  other GNSS software offers.
- The Wails desktop app has distribution problems (macOS
  notarization, Windows signing/SmartScreen) and cannot reach a
  receiver on a headless box.
- The dominant workflow is phased: users explore and configure a
  receiver intensively when setting it up, then leave it alone. The
  right artifact is a commissioning tool that is run when needed, not
  an always-on admin service. Some users want only the configuration
  capability, without running a time server at all.

satpulsewb is run by the user (typically over SSH on the box with
the receiver, or on a laptop reaching a remote receiver via
satpulsed's proxy), prints a URL, and serves a GUI session until
stopped. It is a commissioning tool with a browser UI, not a daemon.

## Relationship to satpulsed

satpulsed is not changed. Receiver config remains a one-shot startup
phase there; satpulsed's own web UI stays read-only (it may later
gain a read-only config view from the startup probe, which is cheap
and safe, but that is not part of this plan). Live touch-ups on a
running system go through a writable `proxy.socket`/`proxy.tcp`, the
same supported path `satpulsetool gps --socket` uses today,
arbitrated by the existing `OutPortLock` write lock. Reset-class
operations are gated off in proxy mode: a USB reset re-enumerates the
device, satpulsed's scan hits EOF and it exits for systemd to restart
it, the proxy connection dies mid-session, and the restart reapplies
the TOML-derived config on top of what the user just did.

Long-term context (not in scope, but the design must not foreclose
it): satpulse as the basis of a vendor GNSS appliance implies an
always-on, on-device admin surface. The intended model there is
declarative: the web UI edits persistent config (TOML as the single
source of truth) and applies by service restart, reusing the daemon's
existing startup config phase as the apply mechanism. The hedges this
plan takes for that future: the UI-to-backend contract splits a
universal core from an optional connection-management capability,
so a third, in-daemon backend -- which owns no port and may apply
declaratively -- can be added behind the same frontend interface
without touching the components; capability gating comes from the
wire contract (`ConfigSupport`, vendor), and the shared components use
semantic design tokens (#284) so a vendor can re-skin without
forking.

## Design: the gps/app/session package

### Background

satpulsed and the desktop GUI wire the same building blocks
(`gpsio.OpenSerial`, `gpsio.Scan`, `bcast.Bcast[scan.Packet]`,
`gpsio.OutPortLock`, `gpscfg.Configure`) in two different shapes. The
daemon treats receiver configuration as a one-shot startup phase and
then hands the packet stream to the dispatcher permanently. The
desktop treats configuration as a repeatable service: a single
long-lived `packetWorker` goroutine owns the packet subscription and
alternates between dispatching messages to the UI and running
`gpscfg.Configure` inline on request, holding the `OutPortLock` for
the duration.

That second shape -- an interactive session with a receiver: connect,
probe, configure, send messages, monitor, disconnect -- is what both
GUI frontends need, and it is currently locked inside the desktop
module behind Wails-specific plumbing. The Wails coupling is thin:
every event the frontend consumes is already a name plus a
JSON-serializable payload (`runtime.EventsEmit`), and every method
returns JSON-serializable values. The extraction replaces that thin
layer with interfaces.

### Package placement

`gps/app/session`, application layer. It orchestrates goroutines and
logs, and everything it needs lives under `gps/` (`gpsio`, `gpscfg`,
`bcast`, `stream`, `msgfile`, `scan`, `gpsprot`, `gpsdecode`). It
needs nothing from `time/`. It must be importable both by the desktop
module (via its existing `replace`) and by `cmd/satpulsewb` in the
main module, which rules out `gps/internal/`.

The package imports `gps/gpsreg` directly and takes the vendor as a
`Connect` parameter. This bends the internals.md convention that the
command-line layer interacts with `gpsreg` and injects
implementations downward ("generally"), and is a deliberate choice:
every conceivable consumer of this package is a GUI shell that wants
the full registry linked, the vendor is naturally a per-session
runtime choice (a `--vendor` flag mirroring `satpulsetool gps
--vendor`), and reconnection needs to rebuild the stateful packet
processors itself. An injected protocol-source interface can be
introduced later if a registry-free consumer ever materializes.

### Construction

```go
type Options struct {
    ProbeTimeout time.Duration // default 15s
    MaxRetries   int
    PacketLog    io.Writer // optional JSONL packet log (nil to disable)
}

func New(lg *slog.Logger, sink Sink, opts Options) *Session
```

### Event sink

The Wails/SSE abstraction. One method, with type safety kept in
exported payload structs:

```go
type EventName string // "gps:state", "gps:receiver", "gps:msg", ...

type Event struct {
    Name EventName
    Data any // JSON-serializable; one of the payload types below
}

// Sink delivers events to the UI transport. Called from session
// goroutines; Emit must not block (drop or buffer).
type Sink interface {
    Emit(Event)
    // Wants reports whether anyone is listening for this event.
    // The session uses it to suppress expensive high-rate streams
    // (gps:packet). The desktop sink returns true unconditionally.
    Wants(EventName) bool
}
```

All event payload types move from `app.go` into the package and
become the documented wire contract: `ConnState` (gps:state),
`ReceiverEvent`, speed (gps:speed), `LogEvent`, `MsgEvent`,
`TimeMsg` (gps:time), the PVMsgBundle (gps:epochPVT),
`NMEAPositionEvent`, `MsgSendEvent`, `ResponseEvent`, `CorrEvent`,
`BaseARPEvent`, `gpsio.PacketLogEntry` (gps:packet).

The package also exports the slog bridge currently implemented by
`eventHandler` in app.go:

```go
// NewLogHandler returns an slog.Handler that mirrors records to the
// sink as gps:log events and forwards them to base.
func NewLogHandler(sink Sink, base slog.Handler) slog.Handler
```

### Transport opener

`Connect` cannot open the port once and be done, because a reset over
USB re-enumerates the device: the session must be able to re-open its
transport by itself. The opener is the seam:

```go
// Opener opens (and re-opens) the connection to the receiver.
type Opener interface {
    Open(ctx context.Context) (conn gpsio.Conn, speed int, err error)
    // Socket reports a proxy connection: sets ConfigOptions.Socket
    // for gpscfg.Configure and gates reset operations in the UI.
    Socket() bool
}
```

Provided implementations: `SerialOpener{Device, Speed}` (its Open can
wait for the device node to reappear, which is where the
re-enumeration handling lives), `SocketOpener{Path}`, and later
`TCPOpener{Addr}` (needs TCP dialing in gpsio; NetConn already
handles unix sockets).

### Methods

The current exported App methods with two systematic changes: plain
Go errors instead of `Result{OK, Error}` (Result was a Wails-ism; the
Wails shell wraps errors back into Result, the HTTP shell maps them
to status codes), and file acquisition split out of the msg-file path
so native dialogs stay in the shell.

```go
// Lifecycle. Connect is asynchronous: progress arrives as events.
func (s *Session) Connect(op Opener, vendor gpsreg.Vendor) error
func (s *Session) Disconnect()

// Snapshots, for Wails HMR re-sync and late-joining browser tabs.
func (s *Session) State() ConnState
func (s *Session) Receiver() ReceiverEvent
func (s *Session) Speed() int
func (s *Session) CorrectionsState() CorrEvent

// Configuration (the packetWorker configCh path, unchanged inside).
func (s *Session) ReadConfig(ctx context.Context) (*gpsprot.ConfigProps, error)
func (s *Session) ApplyConfig(ctx context.Context, target *gpsprot.ConfigTarget) error
func (s *Session) SignalCatalog(gs gpsprot.GNSSSet) map[string][]string

// Message files. The shell obtains the *msgfile.Parsed however it
// likes: native dialog (desktop), library path or upload (web).
func (s *Session) SetMsgFile(mf *msgfile.Parsed) []MsgFileTag
func (s *Session) SendMsgFile(tag, port string, save bool) error
func (s *Session) CancelMsgSend() error

// Corrections.
func (s *Session) StartCorrections(src CorrectionSource) error
func (s *Session) StopCorrections() error

// Stateless helpers become package functions.
func DecodePacket(formats []gpsprot.PacketFormat, data []byte, out bool) (*gpsdecode.DecodeResult, error)
```

The geodesy helpers (`ECEFtoLLH`, `LLHtoECEF`, `CheckOnEarth`) do not
move here; they are thin wrappers over `gps/lib/geopos` and each
shell exposes them directly (or the web UI reimplements them
client-side).

### What moves, what stays

Moves into `gps/app/session` (the bulk of app.go): the App struct
fields and connection state machine, `packetWorker`, `sendWorker` and
its coordinator and correlator plumbing, `ggaMonitor`,
`msgHandler`/`timeEmitter` (EventsEmit calls become sink.Emit),
`handleMsgPacket`, corrections start/stop, all event payload types.

Stays in `desktop/`: Wails bindings (thin wrappers calling the
session and adapting error to Result), `OpenFileDialog`/
`MessageDialog`, `logdir` and log-file wiring, the sink
implementation, darwin/windows serial enumeration.

### New behavior (not pure extraction)

Most of the extraction is relocation plus renaming EventsEmit to
sink.Emit. Three things are genuinely new and need design care in
review:

- Reconnect/re-enumeration: a reset over USB makes the device node
  vanish and return (possibly renamed). The session gets a
  reconnecting state (new `ConnState` value): on read failure while a
  reset-bearing operation is in flight, poll the opener, re-open,
  re-probe, resume. This also fixes the desktop's existing
  read-error-disconnect gap
  (webui/packages/workbench/plan/issues.md), where an unplugged
  device leaves the app stuck in connected state.
- Reset gating over proxy connections, driven by `Opener.Socket()`
  (see Relationship to satpulsed above for why).
- `Wants` gating for the gps:packet stream, so a web client only
  pays for packet streaming while the Packets tab is open.

### Testing

The session is goroutine orchestration around interfaces that already
have test seams: `Opener` and `gpsio.Conn` can be faked, packet
streams scripted, and the sink captured. Unit tests cover the state
machine (connect/probe/configure/send/disconnect transitions,
reconnect on device disappearance, config request serialization under
the port lock) without hardware. The gpscfg layer below is already
tested; these tests target the orchestration.

## Design: serial enumeration and CGO

`desktop/serialenum` moves as-is (still using
`go.bug.st/serial/enumerator`) to `gps/lib/serialenum`. This adds
go.bug.st to the main go.mod, which is acceptable because satpulsed
and satpulsetool must not import the package, so their binaries link
none of it and their Linux builds remain CGO-free. Only satpulsewb
and the desktop shell import it.

The dependency does not cost libc-freedom: go.bug.st's only cgo is
in its darwin enumerator (IOKit); the Linux side is pure Go (a
`/dev` name scan including platform UARTs like ttyAMA, with USB
metadata from `/sys/class/tty/*/device`), so the satpulsewb Linux
binary is statically linked as-is. An earlier plan to hand-roll the
Linux enumeration (originally justified as restoring libc-freedom,
which was never actually at risk) has been dropped; the one thing a
hand-rolled version would add is `/dev/serial/by-id` human-readable
port names, which can be done inside `gps/lib/serialenum` any time
without a dedicated phase.

## Design: the satpulsewb binary

`cmd/satpulsewb`, in the main module, command-line layer. No new
external dependencies: HTTP server, SSE, and token auth are stdlib.
The frontend is embedded as checked-in built assets, the same
`//go:generate npm ... run embed` technique satpulsed uses for
its dashboard (`time/internal/web`, #283); `go build` never needs
npm. satpulsewb bundles its own frontend, so it embeds its own
assets rather than reusing satpulsed's package.

### Command line

Flag parsing follows the gpscmd pattern (pflag, a flagVars struct,
ContinueOnError); connection flags reuse the satpulsetool gps names
and help strings exactly.

```
satpulsewb [-L HOST:PORT] [-T] [--packet-log PATH] [--no-open]
           [-d DEVICE [-s SPEED]] [--vendor NAME]
```

- No arguments just works: bind all interfaces, canonical default
  port (falling back to an OS-picked port if taken), per-run
  generated token, and one printed URL per non-loopback interface
  address with the token as a query parameter
  (`http://192.168.1.40:PORT/?t=XYZ`). The SPA stores the token,
  strips `?t=` from the URL bar, and sends it on every request and
  SSE connection. The per-run token is the only auth model: no
  user-specified token value, no persistent token state, no TLS in
  the first version. On a network the user does not trust, the
  answer is `-L localhost:PORT` plus an ssh tunnel, which the man
  page documents.
- `-L`/`--listen HOST:PORT` takes control of the bind address, and
  with an explicit port a bind failure is an error, no fallback
  (the user may have an ssh tunnel pointing at that port). Since
  the typical `-L` workflow is an ssh tunnel, `-L` also disables
  the token; `-T`/`--token` turns generated-token auth back on
  (`-T` without `-L` is accepted and redundant). Serving without a
  token on a non-loopback address prints a notice.
- `-d`/`--serial-device` auto-connects at startup;
  `-s`/`--device-speed` stays optional as in satpulsetool. Connect is
  asynchronous, so a browser that arrives later catches up from
  the snapshot endpoints. Without `-d` the session starts
  disconnected and the user connects from the UI (device dropdown
  from `gps/lib/serialenum`).
- `--vendor` (empty = autodetect) selects the vendor for every
  connect in the session, at startup or from the UI, and does not
  require `-d`. It is an expert knob and stays command-line only:
  the UI has no vendor control. (An earlier revision had a vendor
  dropdown in the connect bar; it was removed as clutter serving a
  rare case.)
- `--packet-log PATH` mirrors satpulsetool and wires the session's
  `Options.PacketLog`.
- Browser auto-open by default (deferred to phase 7, its own PR),
  gated on a local interactive GUI session. The gate is display
  presence, not receiver locality:
  sshing into the box with the receiver must not open a browser,
  while running at a local desktop should, even when the receiver is
  remote over a later proxy transport. `SSH_CONNECTION` or `SSH_TTY`
  in the environment is a universal veto; otherwise Linux
  additionally requires `DISPLAY` or `WAYLAND_DISPLAY`, macOS needs
  only not-remote (the console user always has the window server),
  and Windows normally has a desktop. What opens is the loopback URL
  (`http://127.0.0.1:PORT/`, with the per-run token appended when
  there is one), orthogonal to the bind: the default all-interfaces
  bind already includes loopback, so the per-address LAN URLs still
  print and stay reachable, and auto-open only adds the local tab. A
  small per-OS launcher does it (`open`/`xdg-open`/`rundll32`, no new
  external dependency), fired after the listener is bound,
  non-blocking, with launch failure logged at debug only since the
  printed URL is the fallback. `--no-open` suppresses it, for a local
  desktop where the tab is unwanted or when the tool is scripted (the
  detection has no positive override; the printed URL covers the rare
  case where it guesses headless but a browser is in fact reachable).
  This supersedes the earlier "no browser auto-open" decision, which
  assumed the primary flow was ssh to a headless box; macOS is now
  the lead desktop platform.
- `--socket` and `--tcp` are deferred to phase 10 (see Transports
  and the Delivery section); `--msg-dir` arrives with the phase-6
  message-file PR.

### HTTP API

Thin adapters over `gps/app/session`:

- POST endpoints for the session methods (connect, disconnect,
  read-config, apply-config, send-msg-file, cancel, corrections
  start/stop, decode-packet). JSON bodies use the existing wire
  shapes (`gpsprot.ConfigTarget`/`ConfigProps` custom marshaling,
  `CorrectionSource`, ...). Errors map to status codes plus a JSON
  error body.
- GET snapshot endpoints (state, receiver, speed, corrections state,
  signal catalog, ports) for late joiners, mirroring the session's
  snapshot methods.
- One SSE endpoint multiplexing all session events (the satpulsed
  `/sse` pattern, using `time/lib/sse`). The server keeps a
  latest-event-per-name cache to prime new subscribers, so a second
  browser tab starts consistent. The high-rate gps:packet stream is
  subscription-gated via the sink's `Wants` mechanism: streamed only
  while a client has the Packets tab open.

Multiple simultaneous clients are tolerated (same token, events
broadcast, writes serialized by the session); this is a single-user
tool, not a multi-user system.

### Message files

The desktop's native file dialog is replaced by, in order of
priority:

1. A library browser: an endpoint that walks the installed message
   file tree (`/usr/share/satpulse/gpsmsg`, falling back to a
   `--msg-dir` override for source-tree use) and returns files with
   their tags and descriptions (via `msgfile` tag parsing), so the UI
   presents a browsable catalog rather than a path prompt. Selection
   is by relative path within the library root, sanitized against
   path traversal (the token holder is choosing server paths).
2. An upload endpoint accepting TOML bytes for ad-hoc files, parsed
   server-side with the same validation.

The desktop app can later adopt the same library browser, keeping the
native dialog for ad-hoc files.

### Frontend

Builds on the #283 workspace. The desktop frontend's components move
into the workspace verbatim in phase 1 (below); this plan adds:

- A transport interface for the UI, shaped so a third backend can be
  retrofitted later without touching the components: it must not
  assume exclusive ownership of a serial port. Split it into a
  universal core -- snapshots, config read/apply, message-file send,
  event subscription -- that the config panel and all tabs depend
  only on, and an optional connection-management capability --
  connect/disconnect, port listing, and the
  connecting/reconnecting connection states -- implemented only by
  the direct-serial and proxy backends. The two implementations here
  are the existing generated wailsjs bindings (desktop) and fetch+SSE
  (satpulsewb); a later in-daemon backend (the appliance admin
  surface under Relationship to satpulsed) has a permanently
  connected receiver and no connection management, and its apply may
  be declarative (edit persistent config, apply by service restart)
  rather than an imperative Configure run with live progress. This
  overturns webui/packages/workbench/plan/shared-webui.md's assumption
  that the config panel stays desktop-specific; that file is corrected
  in phase 4.
- A satpulsewb entry package in the workspace (`workbench-http`:
  index.html, token handling, fetch/SSE transport wiring), whose
  Vite build output is what `cmd/satpulsewb` embeds.
- Web replacements for native chrome: MessageDialog error popups
  become inline notices; BrowserOpenURL becomes window.open (already
  anticipated as a callback prop in the shared map component plan).
- The geodesy helpers (`ECEFtoLLH` etc.) become either small POST
  endpoints or a TypeScript reimplementation; decide at
  implementation.

### Transports

- Serial (`SerialOpener`): primary; the tool owns the port; full
  feature set including resets, with re-enumeration handled by the
  session.
- Unix socket (`SocketOpener`): touch-ups through a running
  satpulsed's `proxy.socket`; reset ops gated off.
- TCP (`TCPOpener`): same, via `proxy.tcp`, for reaching a headless
  box from a laptop. Requires adding TCP dialing to gpsio. Known
  caveat from webui/packages/workbench/plan/issues.md
  (tcp-connect): inter-packet idle detection is unreliable over
  TCP, so the NMEA satellite buffer falls back to its
  key-detection flush and the satellite display lags one cycle;
  configuration is unaffected.

Serial ships in phase 3; socket and TCP land together in phase 10.
The session side of socket is already done (SocketOpener,
reset gating), but the UI has no capability gating for proxy
connections yet -- reset-class controls must be hidden or disabled,
driven by the wire contract -- and TCP additionally needs the gpsio
dialing.

### Build, packaging, docs

- Makefile and bsd-build.sh targets for satpulsewb. Recent term work
  suggests Windows support is coming to the main module; satpulsewb
  inherits whatever platforms gpsio/term support, with Linux the
  primary target.
- Packaging: ride the existing deb/rpm or a subpackage -- decide at
  packaging time; nothing architectural depends on it.
- Man page `satpulsewb(1)`; NEWS.md entry in the same change as the
  implementation (release notes rule).
- docs/internals.md entries for `gps/app/session`,
  `gps/lib/serialenum`, `cmd/satpulsewb`, and the embed package.

## Delivery: branching strategy and phases

Constraints: desktop-gui stays a long-lived branch (the separate
module is a Wails tax we do not extend); the phases are developed as a
stack of individually reviewable PRs, each branching off the previous
phase's branch, and none is merged to master before the next phase
starts; the history of the desktop frontend components must be
preserved.

Principle: exactly one PR (phase 1) carries a merge of desktop-gui.
Because every later phase branches off the phase-1 branch (directly or
transitively through the stack), that merge is in each one's ancestry,
so they reference the desktop history for free: `git log` on deleted
paths keeps working, and content is recoverable via the
`desktop-gui-import` tag with `git show <ref>:<path>` or
`git restore --source=<ref>`. This holds through the stack, with
nothing needing to land on master first.

Landing: the stack is reviewed as a series and merged bottom-up onto
master (phase 0, then 1, then 2, ...) once it is ready; phases are not
landed one at a time as they are written. Until then each phase's PR
targets the previous phase's branch, so its diff shows only that
phase's own changes.

### Phase 0 (prerequisite): web toolchain (#283)

As planned in [web-toolchain.md](web-toolchain.md). No desktop
involvement; creates the `webui/` workspace. Its PR (on the
`web-toolchain` branch) is opened for review and stays open as the
base of the stack; phase 1 branches off it rather than off master.

### Phase 1: webui import (one PR; the history carrier)

Branch off `web-toolchain` (not master): phase 0's PR stays open, so
phase 1 is a stacked PR on top of it. Sync desktop-gui with master
one last time. `git merge desktop-gui`; tag the merged head
`desktop-gui-import`. Then:

- pure `git mv` commits (no content edits, so rename detection binds
  `git log --follow`): frontend components into a new workspace
  package (`webui/packages/workbench`, paired with the read-only
  `dashboard`), `desktop/serialenum` -> `gps/lib/serialenum` as-is,
  and `desktop/plan` -> `webui/packages/workbench/plan` so the design
  docs travel with the code they describe;
- adaptation edits in separate commits: `gps/lib/serialenum` adds
  `go.bug.st` to the main `go.mod`; the moved plans' escaping links
  are re-pointed; a provenance note is added to the workbench plan
  README;
- prune the rest of `desktop/` (the Wails shell, build tooling,
  logdir, module go.mod).

The workbench package lands parked: it still imports the Wails-generated
wailsjs bindings, so it is deliberately not registered in the root
`workspaces` array and is not compiled. Registering it and replacing the
wailsjs imports with the transport interface is phase 3 work; here it is
purely the arriving history, and the build stays green because the
parked package is excluded from it.

Import components verbatim even where they overlap with dashboard
components (two sky views, etc.); unification is #284's job, not this
PR's. Net diff vs its `web-toolchain` base: the added webui files and
the serialenum package. The commit list is long -- that is the history
arriving.

### Phase 2: gps/app/session (one PR)

Branch off the phase-1 branch, continuing the stack. Phase 1 is a real
prerequisite: it creates the `desktop-gui-import` tag that commit 1
below is resurrected from and verified against, and puts that history
into the stack's ancestry before the derived code lands.

The PR's aggregate diff against its base necessarily shows
`gps/app/session` as new files (the phase-1 branch no longer contains
`desktop/app.go`). Reviewability comes from the per-commit structure,
so the PR must be reviewed commit by commit, not from the
files-changed view:

```
git restore --source=desktop-gui-import -- desktop/app.go ...  # commit 1: verbatim
git mv desktop/app.go gps/app/session/session.go               # commit 2: pure move
# commits 3..n: the extraction edits (sink, errors, Opener, reconnect)
```

Commit 1 renders as a large file addition but is not read: it is
verified byte-identical with `git diff desktop-gui-import <commit1>
-- desktop/app.go` printing nothing. Commit 2 renders as a pure
rename with no content diff. Commits 3..n are the only commits that
need real review: ordinary diffs of the extraction edits against the
resurrected original. Record the provenance ("derived from
desktop/app.go") in the commit message, since the delete/re-add
breaks `git log --follow`. Includes
the session unit tests and internals.md entries. The desktop app is
not rewired in this phase; that is phase 4.

### Phase 3: cmd/satpulsewb (one PR)

The key phase: the binary (server, token auth, SSE sink, embed
bridge) plus the fetch/SSE transport implementation and entry
package in the workspace, carrying the whole existing UI. Every tab
transfers as-is -- the native msg-file dialog is the only frontend
call with no web counterpart -- so config, monitor, packets, and
corrections all land here; the Messages tab is hidden until phase 6.
Serial transport only (see Transports). This alone is a usable
tool, so the NEWS.md entry and man page ride this PR.

### Phase 4: rework desktop-gui on top (branch work, no master PR)

On the desktop-gui branch: merge the tip of the stack (the phase-3
`satpulsewb` branch), then:

- delete the branch's local copies of the frontend components in
  favor of the workspace packages (the `file:` dependency mechanism
  from #283);
- rewrite `desktop/app.go` as a thin Wails shell over
  `gps/app/session`: sink implementation mapping Emit to
  runtime.EventsEmit, bindings adapting errors to Result, dialogs,
  logdir, darwin enumeration;
- keep the native dialog for message files (the library browser
  arrives in phase 6 and can be adopted at a later merge);
- correct webui/packages/workbench/plan/shared-webui.md (the
  config panel is shared after all).

Message handling background: this phase needs nothing from phase 6,
because message files are already fully supported on both sides of
the seam. The session exports the whole msg-file surface
(`SetMsgFile(*msgfile.Parsed)`, `SendMsgFile`, `CancelMsgSend`,
extracted in phase 2), and the workbench frontend carries the
complete Messages tab (msgfile-panel.tsx), shown whenever the
transport provides the optional `msgFile` capability. That
capability's `loadMsgFile(): Promise<MsgFileInfo | null>` is exactly
the native-dialog shape: the Wails shell implements it as
OpenFileDialog, parse the file with `msgfile`, `SetMsgFile`, return
path and tags (null on cancel). Phase 6's endpoints, catalog UI, and
`--msg-dir` are the web-side replacement for the dialog, not a
dependency. This phase is therefore also the first shell to exercise
the session's msg-file send path (sendWorker, correlator, response
events), which satpulsewb cannot reach until phase 6 adds its
endpoints. Expected interaction with phase 6: the library catalog
may extend `MsgFileTransport`; the desktop picks that up in a later
routine merge, which is when the adopt-the-library-browser option
above becomes available.

After this phase, once the stack has landed on master, the branch's
delta over master is small: one module directory containing a thin
shell.

### Phase 5: replay smoke tests (one PR)

Black-box smoke tests of the satpulsewb binary, feasible as soon as
phase 3 exists: replaying a packet log through a FIFO with
`satpulsetool pack --realtime` drives the whole monitor path with no
hardware, because gpscfg skips probing on a read-only port --
connect succeeds and passive detection takes over. A pty replay
works too: the probe runs, goes unanswered, and connect still ends
in passive detection (the session tolerates ErrNoProbeResponse), so
write-path scenarios need no responding receiver.

The tests are scenarios in the existing `smoketest/` suite, not a
sibling runner: the program under test becomes a scenario-declared
dimension of `run.py` (default satpulsed), captured in one
per-program module behind a small protocol, the same way
`platform_api.py`/`platform_unix.py` already capture the OS
dimension. The program seam owns exactly what differs between the
two programs: input preparation (satpulsed renders a toml template
and derives its helper peers from the rendered config; satpulsewb
takes a flag list, with the same `${SATPULSE_TEST_*}` substitution,
and declares its peers explicitly), the start command, readiness
(config-derived listeners vs one HTTP port plus parsing the printed
URL and token), the base allowed-error list for the daemon-log
scan, and shutdown expectations. Everything else stays shared:
port-block allocation, run dirs, the transport layer (FIFO/pty,
write capture, disconnect), the replay lifecycle, the fake peers,
and the parallel executor. Scenario families stay organized by
feature and hold scenarios for both programs side by side; the
workbench corrections scenarios land beside the daemon's
`stream/pull-*` ones and reuse the family's captured-serial-writes
RTCM check as-is.

Checks at smoketest depth: startup and the printed URL/token, auth
enforcement and the `-L`/`-T` token modes, snapshot endpoints
populating as the replay flows, SSE delivery and priming,
packet-stream gating driven by a scripted SSE client, clean
shutdown (auto-open gating checks arrive with the phase-7 auto-open
PR). Corrections rides this phase, mirroring the daemon's
stream/pull scenarios: a pty scenario with `fakesource.py` as the
caster checks the corrections state transitions over SSE,
gps:corrpacket and gps:basearp delivery, the RTCM written back to
the port (detection-probe writes filtered by tag, as today), the
VRS refusal while no fix exists, and the GGA upload once one does.
Device loss inverts SELF_SHUTDOWN: on a transport disconnect
satpulsewb must keep running and emit the disconnected state over
SSE rather than exit.

Includes recording two purpose-built F9P fixtures: a rover (the
message set the workbench displays, plus GGA fixes for the VRS
scenario) and a static base during survey-in (RTCM in the packet
mix, survey events exercising the priming of slow gps:msg kinds),
each long enough at its replay factor to outlast the slowest check.
The existing multi-vendor logs under `gps/testdata/packets/` serve
as secondary fixtures, since detection is passive.

### Phase 6: message files (one PR)

Message-file loading, the one piece of genuinely new UI: the
library and upload endpoints (including the path-traversal
sanitization), the browsable catalog UI, `--msg-dir`, and the
msg-file send/cancel endpoints; un-hides the Messages tab.

### Phase 7: browser auto-open (one PR)

Browser auto-open as specified under Command line above: the
local-GUI-session gate, the per-OS launcher, `--no-open`, and the
smoke-test checks for the gating (environment manipulation in the
phase-5 scenarios) ride along.

### Phase 8: Playwright browser tests (one PR)

Branches off the phase-7 branch, continuing the stack (phase 4 is
desktop-gui branch work, outside it).
DOM-level journeys in a real browser, against the same launch and
replay fixtures as phase 5: a small `@playwright/test` suite in the
webui workspace whose setup starts satpulsewb on a FIFO replay.
Journeys: the SPA boots and the token is consumed and stripped from
the URL bar; satellites and position render and advance; the Packets
tab starts and stops the packet stream; a second tab late-joins
consistent from the event cache; the stale-token notice; re-priming
after a server restart. After phase 6 so a Messages tab journey can
ride along, and after the desktop rework (phase 4) so the components
are serving both shells before journeys pin their DOM. Kept to a
handful of shallow journeys: wire-level assertions stay in the
phase-5 scenarios, which need no browser or npm.

### Phase 9: simulator config tests (one PR)

Extends both harnesses from the monitor path to the config path,
using the u-blox receiver simulator (#362,
[ublox-sim.md](ublox-sim.md)), which is developed in parallel on its
own branch off master. In smoketest terms the simulator is a new
packet provider, not a transport: what plays the receiver behind
the serial transport becomes another scenario-declared dimension,
orthogonal to the program under test -- either a paced `pack
--realtime` replay of a recorded log (whose one-replay-per-lifetime
invariant and wait-replay backstop belong to this provider, not to
the suite) or the interactive simulator, which answers probes and
config writes and so implies a read-write pty. The dimensions
compose: satpulsed x simulator smoke-tests the daemon's startup
config phase, which no replay can reach. The smoketest scenarios
and phase-8 journeys gain
config checks: probe identifies the personality, the config panel
populates from ReadConfig, an apply round-trips and a re-read shows
the change, and enabling a message makes it appear in the packet
stream. Not part of the stacked-PR series: it needs the simulator on
master, so it runs after the stack and the simulator have both
landed.

### Phase 10: proxy transports (one PR)

Proxy transports: `--socket` (SocketOpener is already done in the
session, including reset gating) and `--tcp` (needs TCP dialing in
gpsio), plus the UI capability gating for proxy connections --
exposing socket-ness through the wire contract and hiding or
disabling reset-class controls. Not part of the stacked-PR series:
deliberately last, an ordinary PR off master once the stack has
landed.

- Names: the embed package location (own package vs go:embed directly
  in cmd/satpulsewb).
- macOS enumeration in satpulsewb: keep go.bug.st, or glob
  `/dev/cu.*` and avoid cgo there too.
- The default port number.
- Whether `satpulsetool gps` should print a hint about satpulsewb
  for discoverability.
