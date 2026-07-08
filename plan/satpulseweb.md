# satpulseweb: serve experimental desktop GUI as web app (#357)

A new binary `cmd/satpulseweb` that serves the desktop GUI as a web
app: an HTTP server with an embedded single-page frontend, driven by
an application core extracted from the desktop backend into a new
package `gps/app/session`. The top priority is device-independent
receiver configuration through a GUI that requires no knowledge of
the receiver's protocol; the full desktop feature set (monitor,
packets, message files, corrections) comes along because it flows
through the same core.

The work is delivered in phases, each an individually reviewable PR
on master, with the Wails desktop app reworked on top at the end (on
the desktop-gui branch). The phase structure and the branching and
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

satpulseweb is run by the user (typically over SSH on the box with
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
plan takes for that future: the UI-to-backend contract is
transport-neutral (a third, in-daemon backend can be added behind the
same frontend interface), capability gating comes from the wire
contract (`ConfigSupport`, vendor), and the shared components use
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
module (via its existing `replace`) and by `cmd/satpulseweb` in the
main module, which rules out `gps/internal/`.

The package imports `gps/gpsreg` directly and takes the vendor as a
`Connect` parameter. This bends the internals.md convention that the
command-line layer interacts with `gpsreg` and injects
implementations downward ("generally"), and is a deliberate choice:
every conceivable consumer of this package is a GUI shell that wants
the full registry linked, the vendor is naturally a per-connection
runtime choice (a connect-form dropdown mirroring `satpulsetool gps
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
  read-error-disconnect gap (desktop/plan/issues.md), where an
  unplugged device leaves the app stuck in connected state.
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
none of it and their Linux builds remain CGO-free. Only satpulseweb
and the desktop shell import it.

A later phase reimplements the Linux side by hand -- pure stdlib,
walking `/dev/serial/by-id` symlinks (human-readable names) and
`/sys/class/tty/*/device` (USB metadata, and platform UARTs such as
ttyAMA0 on a Pi, which the dependency's USB-centric enumeration
misses) -- and drops go.bug.st from the Linux build. That is what
makes the satpulseweb Linux binary CGO_ENABLED=0/libc-free like its
siblings. The macOS binary may keep cgo (IOKit or go.bug.st) for
proper display names; libc-freedom is a Linux property.

## Design: the satpulseweb binary

`cmd/satpulseweb`, in the main module, command-line layer. No new
external dependencies: HTTP server, SSE, and token auth are stdlib.
The frontend is embedded as checked-in built assets, the same
`//go:generate npm ... run embed` technique satpulsed uses for
its dashboard (`time/internal/web`, #283); `go build` never needs
npm. satpulseweb bundles its own frontend, so it embeds its own
assets rather than reusing satpulsed's package.

Invocation sketch:

```
satpulseweb [--listen addr] [--token STRING]
            [-d DEVICE [-s SPEED] | --socket PATH | --tcp HOST:PORT]
```

- Default bind is localhost. Binding non-locally is an explicit
  `--listen` choice.
- A bearer token is always required: `--token` to specify, otherwise
  generated and printed at startup as part of the URL
  (`http://host:8080/?t=XYZ`); the SPA stores it and sends it on
  every request/SSE connection. When run on the user's own machine,
  optionally open the browser.
- `--cert`/`--key` for TLS can be added when serving across a LAN;
  deliberately not required for the first version given the
  ephemeral, token-gated lifecycle.
- Connection flags mirror `satpulsetool gps`; the connect form in the
  UI can also drive connection interactively (device dropdown from
  `gps/lib/serialenum`, vendor dropdown mirroring `--vendor`).

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

- A transport interface for the UI (read-config, apply-config,
  send-msg-file, snapshots, event subscription) with two
  implementations: the existing generated wailsjs bindings, and
  fetch+SSE. This overturns desktop/plan/shared-webui.md's assumption
  that the config panel stays desktop-specific; that file is
  corrected in phase 5.
- A satpulseweb entry package in the workspace (index.html, token
  handling, fetch/SSE transport wiring), whose Vite build output is
  what `cmd/satpulseweb` embeds.
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
  caveat from desktop/plan/issues.md (tcp-connect): inter-packet idle
  detection is unreliable over TCP, so the NMEA satellite buffer
  falls back to its key-detection flush and the satellite display
  lags one cycle; configuration is unaffected. TCP can slip to a
  follow-on if phase 3 ships serial+socket only.

### Build, packaging, docs

- Makefile and bsd-build.sh targets for satpulseweb. Recent term work
  suggests Windows support is coming to the main module; satpulseweb
  inherits whatever platforms gpsio/term support, with Linux the
  primary target.
- Packaging: ride the existing deb/rpm or a subpackage -- decide at
  packaging time; nothing architectural depends on it.
- Man page `satpulseweb(1)`; NEWS.md entry in the same change as the
  implementation (release notes rule).
- docs/internals.md entries for `gps/app/session`,
  `gps/lib/serialenum`, `cmd/satpulseweb`, and the embed package.

## Delivery: branching strategy and phases

Constraints: desktop-gui stays a long-lived branch (the separate
module is a Wails tax we do not extend); everything else lands on
master as individually reviewable PRs; the history of the desktop
frontend components must be preserved.

Principle: exactly one master-bound PR carries a merge of desktop-gui
(phase 1). Once merged, all desktop-gui commits are ancestors of
master, so every later phase references that history for free:
`git log` on deleted paths keeps working, and content is recoverable
via `git show <ref>:<path>` or `git restore --source=<ref>`.

### Phase 0 (prerequisite): web toolchain (#283)

As planned in [web-toolchain.md](web-toolchain.md). No desktop
involvement; creates the `webui/` workspace. Its PR (on the
`web-toolchain` branch) is opened for review but deliberately not
merged into master before phase 1 starts, so phase 1 stacks on it.

### Phase 1: webui import (one PR; the history carrier)

Branch off `web-toolchain` (not master): phase 0's PR stays open, so
phase 1 is a stacked PR on top of it. Sync desktop-gui with master
one last time. `git merge desktop-gui`; tag the merged head
`desktop-gui-import`. Then:

- pure `git mv` commits (no content edits, so rename detection binds
  `git log --follow`): frontend components into a new workspace
  package (e.g. `webui/packages/app`), and `desktop/serialenum` ->
  `gps/lib/serialenum` as-is;
- adaptation edits (import paths, removing wailsjs references) in
  separate commits;
- prune everything else the merge brought (all of `desktop/`).

Import components verbatim even where they overlap with dashboard
components (two sky views, etc.); unification is #284's job, not this
PR's. Net diff vs master: the added webui files and the serialenum
package. The commit list is long -- that is the history arriving.

### Phase 2: gps/app/session (one PR)

Fresh branch off master after phase 1 merges. Phase 1 is a real
prerequisite: it fixes the `desktop-gui-import` tag that commit 1
below is resurrected from and verified against, and it puts that
history into master's ancestry before the derived code lands.

The PR's aggregate diff against master necessarily shows
`gps/app/session` as new files (master no longer contains
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
not rewired in this phase; that is phase 5.

### Phase 3: cmd/satpulseweb (one or two PRs)

The binary: server, token auth, SSE sink, embed bridge, msg-file
library endpoints, the fetch/SSE transport implementation and entry
package in the workspace. If one PR is too large, split
monitor-first (server skeleton + connect + monitor/packets tabs),
then config + message files + corrections. NEWS.md entry and man page
ride whichever PR makes the feature usable.

### Phase 4: libc-free Linux enumeration (one PR)

Hand-rolled pure-Go Linux implementation in `gps/lib/serialenum` (see
Design: serial enumeration); drop go.bug.st from the Linux build;
verify the satpulseweb Linux binary builds with CGO_ENABLED=0 and has
no dynamic dependencies.

### Phase 5: rework desktop-gui on top (branch work, no master PR)

On the desktop-gui branch: merge master (which now contains phases
1-4), then:

- delete the branch's local copies of the frontend components in
  favor of the workspace packages (the `file:` dependency mechanism
  from #283);
- rewrite `desktop/app.go` as a thin Wails shell over
  `gps/app/session`: sink implementation mapping Emit to
  runtime.EventsEmit, bindings adapting errors to Result, dialogs,
  logdir, darwin enumeration;
- optionally adopt the msg-file library browser alongside the native
  dialog;
- correct desktop/plan/shared-webui.md (the config panel is shared
  after all).

After this phase the branch's delta over master is small: one module
directory containing a thin shell.

## Open decisions

- Names: the binary (`satpulseweb`), the workspace package for the
  imported components, the embed package location (own package vs
  go:embed directly in cmd/satpulseweb).
- macOS enumeration in satpulseweb: keep go.bug.st, or glob
  `/dev/cu.*` and avoid cgo there too.
- Whether TCP transport ships in phase 3 or as a follow-on.
- Whether `satpulsetool gps` should print a hint about satpulseweb
  for discoverability.
