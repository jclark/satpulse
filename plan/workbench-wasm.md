# SatPulse Workbench in the browser (#486)

Run Workbench entirely in a browser page, with no satpulsed and no satpulsewb
process, as a `GOOS=js GOARCH=wasm` build of `gps/app/session` talking to the
receiver over the Web Serial API.

The prototype now implements connect, probe, configuration, monitoring, packet
decoding, geodesy, and sending embedded message files. Chromium tests exercise
the real Go wasm module against the existing u-blox simulator with Web Serial
mocked. The user also successfully configured a physical Zhongke receiver at
38400 baud. Broader hardware validation remains open. Initial size and startup
measurements are recorded below.

## Shape

Workbench already separates its application core from its shell. `Session` in
`gps/app/session` owns the packet pipeline and delivers events through the
`Sink` interface; it opens its transport through `Opener`; the frontend's
`Transport` interface in `webui/packages/workbench/src/transport.ts` is the
matching seam on the TypeScript side, alongside fetch plus SSE in
`cmd/satpulsewb` and Wails bindings in the desktop app.

The browser build is a third shell:

- `cmd/satpulsewbwasm` binds `Session` to JavaScript through `syscall/js`,
  with a `Sink` whose `Emit` calls a registered JS callback;
- `webui/packages/workbench-wasm` implements `Transport` against those
  bindings and owns the Web Serial stream locks;
- `webui/packages/workbench` supplies the shared UI, with the small capability
  and connection-panel changes listed below.

The Go build is standard Go, not TinyGo. The browser needs `reflect.Select`,
which `gps/app/bcast` uses and TinyGo does not implement, and full reflect for
go-toml in `gps/msgfile`. TinyGo 0.42.0's `reflect.Select` still panics; no usable
full-session TinyGo binary was produced. Reducing download size remains a
reason to investigate TinyGo separately, but it is not a prerequisite here.

## Changes to existing APIs

### Go connection contracts

The independent `session-conn` refactor is committed from `master` and merged
into this branch alongside `non-unix` and `wb-corrections-transport`. It replaces the prototype's anonymous
capability checks with explicit contracts:

| Existing API or implementation | Change | Why it was needed |
| --- | --- | --- |
| `session.Opener.Open` | Return `(session.Conn, error)` instead of `(gpsio.Conn, int, error)` | Require Session's connection capabilities at the opening boundary; obtain speed from the connection itself. |
| `gpsio.SerialOutPort` | Add `SetDetected()` | Express both a baud change and confirmation of the new settings in the contract gpscfg uses. |
| `Session.runConn` and `emitSpeed` | Call `SetPacketLog` and `Speed` directly | Both native and browser serial provide the required methods; missing capabilities are compile-time errors. |
| Correction forwarding | Pass `session.Conn` as a `stream.PacketWriter` | Remove the optional packet-writer assertion and plain-write fallback. |
| `gpscfg.configure` | Assert `gpsio.SerialOutPort` directly | Use baud-change operations for any serial implementation, including browser serial. |
| `session.SocketOpener` | Remove the unused opener | No current shell uses it, so no NetConn adapter is needed. |

The addition is `session.Conn`, defined by the consumer. It embeds
`gpsio.Conn` and `stream.PacketWriter`, adding `Speed()` and `SetPacketLog()`.
The latter transfers ownership of the packet log's output half: complete it
exactly once after the final output write, normally at Stop. Native and wasm
serial satisfy this interface with their existing methods. `session.Sink`,
`gpsio.Conn`, `gpscfg.Configure`, and `gpsio.OpenSerial` retain their contracts.

### Shared shell data

`session.ConnectionInfo` supplies the connection snapshot's common JSON shape;
port selection and retention of speed still belong to each shell.
`Session.MsgFilePreselect` matches an asserted vendor, or otherwise the detected
vendor, against the catalog and returns the catalog's spelling. Both native
and browser shells use it, avoiding divergent selection rules.

### Frontend API changes

Optional capabilities are defined in
`webui/packages/workbench/src/transport.ts`:

- `ConnectionTransport.choosePort?(): Promise<PortInfo | null>` lets the
  browser show its permission picker directly from a user gesture. `null`
  means the user cancelled. `connect(device, speed)` still opens an already
  selected port, and `listPorts()` still supplies the dropdown.
- `Transport.corrections?: CorrectionsTransport`, supplied by the independent
  `wb-corrections-transport` refactor, groups correction snapshots and start/stop
  operations. Its presence enables the tab and panel; the browser omits it.
  The HTTP shell supplies it with its existing endpoints.

The connection panel also adds an optional `onChoosePort?: () => void` prop.
`App` passes it only when the transport implements `choosePort`, records the
returned device, and refreshes the granted-port list.

### Shared UI changes and their reasons

- **Add a device dropdown entry.** The device dropdown lists granted ports
  and ends with `Add a device...`, which closes the dropdown and immediately
  invokes the browser's permission picker. Web Serial requires `requestPort()`
  to run from a user gesture, so this call runs in JavaScript before any
  `await`. Selecting a port grants access; Connect opens it at the chosen
  speed. Cancellation preserves the previous selection. There is no separate
  picker button beside the device control.
- **Device field.** With the picker enabled, the field is read-only and its
  placeholder is `select a device`. Browser device IDs such as `serial:1` are
  page-local handles, not filesystem paths that a user can type. The existing
  dropdown lists previously granted ports, with USB IDs when available.
  An empty browser list says `No devices added`; the native shell retains
  its editable path and existing port list without the add action.
- **Corrections tab and panel.** Both require the optional corrections
  capability. The browser omits it because forwarding is not implemented yet.
  This is a prototype capability limit; future direct Ntrip support can supply
  the capability without changing the UI contract.

## Additions for the browser shell

### Go bindings and serial adapter

`cmd/satpulsewbwasm/main_js_wasm.go` constructs a normal `Session` and exports
`globalThis.satpulseWorkbench` with `init`, `on`, and `call`. Operation arguments,
results, and event payloads use JSON strings. `call` immediately returns a
JavaScript Promise and runs the session operation in a goroutine, allowing the
shared Go/JavaScript event loop to keep servicing serial Promises.

The bindings cover connection and receiver snapshots, connect/disconnect,
configuration, signals, packet decoding, geodesy, and the built-in message
catalog/select/send/cancel operations. They do not expose correction start/stop
operations. `eventSink.Wants` checks whether an event has a subscriber;
JavaScript queues event delivery in a microtask to avoid re-entering Go while
a session operation is emitting an event under a lock.

`cmd/satpulsewbwasm/serial_js_wasm.go` implements the existing `session.Opener`,
`session.Conn`, and `gpsio.SerialOutPort` against Promise-based JavaScript methods.
It adapts reads to the scanner, serializes writes, logs outgoing packets,
retains speed for reconnects, and separates `Stop` from final `Close` cleanup.

The browser URL accepts `?vendor=Zhongke`, equivalent to native Workbench's
`--vendor Zhongke`. The frontend passes the value to the wasm shell's `init`;
Go validates it with `gpsreg.ParseVendor` before mounting the UI. Vendor names
are case-insensitive and aliases such as `CASIC` are accepted. The shell
passes the singleton vendor list to every `Session.Connect`, enabling that
vendor's experimental configuration protocol and selecting its packet formats
and decoding behavior. The asserted vendor also drives message-file
preselection. An omitted or empty parameter keeps automatic detection; an
unknown vendor produces a startup error. No environment-variable fallback is
used in the browser.

### Frontend entry package

`webui/packages/workbench-wasm` contains:

- `src/main.tsx`: a loading message, secure-context/Web Serial checks, startup
  errors, and mounting the shared `App` after the wasm transport is ready.
- `src/wasm-transport.ts`: streaming wasm instantiation, runtime registration,
  the existing `Transport` implementation, and event subscriptions.
- `src/web-serial.ts`: permission selection, granted-port identities,
  reader/writer ownership, a bounded input queue, and serial cleanup.
- Vite/package configuration and an HTML entry loading `wasm_exec.js`.

The browser-only additions do not introduce another receiver protocol or
configuration pipeline. The existing Go session remains responsible for those
operations.

## What the shell does not need

The wasm shell is the thinnest of the three, because most of `cmd/satpulsewb`
exists to survive a network boundary and multiple browser windows, and here
there is one client, born with the module and dying with it:

- no application HTTP server: the deployment server serves static files only;
- no per-run token and no write seat, so `Transport.reclaim` is omitted as it
  is on the desktop;
- no latest-event-per-name cache, which exists to prime late-joining clients;
- `Sink.Wants` degenerates to whether the JS side has registered a callback
  for that event name, which is still worth keeping to gate `gps:packet`;
- no browser launcher: the user opens the static site directly.

Message files come from `msgfile.Builtin`, an embedded zip exposed as an
`fs.FS`. Importing a custom message file is not implemented.

## Web Serial against the existing interfaces

`gpsio.Conn` is small (`Read`, `Write`, `Close`, `Stop`, `Buffered`,
`ReadOnly`, `Direct`, `Drain`, `LocalAddr`, `SetDetected`) and `session.Opener`
is two methods. `session.Conn` composes the I/O contract with Session
requirements, so a Web Serial implementation plus an `Opener` is the natural fit.
`term.Term` is not needed: only `gps/app/gpsio/serial.go` uses it, and on js it
resolves to the stub `non-unix` added.

The adapter makes the following choices:

- **Speed change.** Write the receiver command, wait for estimated 8N1
  transmission, cancel/release the streams, close the port, and reopen at
  the new baud. There is no `tcdrain`; physical drain timing and possible
  DTR/device-reset effects still need hardware validation.
- **`Buffered()`** returns 0 because Web Serial does not expose pending UART
  output bytes. `Drain()` waits for the JavaScript transmission estimate.
- **Blocking `Read`** over a promise-based `ReadableStream` needs a
  `js.FuncOf`-plus-channel bridge. Input is queued in JavaScript, bounded to
  1 MiB and 4096 queued chunks/errors; each received chunk is copied into Go
  and served from a local buffer. Chunks and error markers remain in arrival
  order. An idle read returns a scanner timeout after 100 ms; closure returns EOF.
- **Cleanup.** `Stop` wakes pending reads and detaches logging. `Close` waits
  for writes and then cancels the reader, releases stream locks, and closes
  the port. Repeated JavaScript closes share one cleanup Promise.
- **Serial faults.** Framing, parity, break and buffer-overrun faults release
  the failed reader and acquire the replacement readable stream. They reach Go
  as `gpsio.SerialError` with the existing `Temporary()` and `SerialFraming()`
  contracts, preserving scanner recovery and detection timing. Browser faults
  have flags but no hardware error counts. Terminal faults and input overflow
  end the connection after previously queued input is delivered.
- **Port ownership.** Each port remains reserved through opening, use, baud
  changes and close. New connections wait for superseded attempts to finish
  cleanup. The Go opener's context aborts its JavaScript wait; because Web
  Serial cannot abort the underlying open, a late completion is closed before
  the reservation is released.
- **Runtime failure.** The browser shell monitors Go throughout its lifetime,
  rejects pending and future calls on exit, closes JavaScript serial resources,
  and replaces the UI with a visible error. Promise rejection reasons retain
  their identity and accept primitive values without panicking the Go runtime.

## Prototype limits

- **PPS.** Web Serial exposes control signals only through `getSignals()`
  (`dataCarrierDetect`, `clearToSend`, `dataSetReady`, `ringIndicator`), an
  async poll with no edge event and no timestamping.
- **Ntrip caster.** No listening sockets.
- **Correction forwarding.** The existing TCP and Ntrip v1 sources dial raw
  TCP and are not connected to the wasm shell. Direct browser Ntrip v2 support
  is separate future work below.
- **Custom message files.** Only the embedded catalog is currently exposed.

Connect, probe, configure, send message files and monitor are all local and
unaffected, and are the bulk of what Workbench is for.

## Additions for building and validation

This branch starts from `non-unix`, which is already an ancestor. Merge that
prerequisite again rather than cherry-picking if it moves.

`Makefile.wasm` builds with standard Go for `js/wasm`, using `-trimpath` and
`-ldflags '-s -w'`, copies the matching toolchain's `wasm_exec.js`, and assembles
the site under `out/workbench-wasm`. `make -f Makefile.wasm compress` additionally
writes gzip/Brotli assets and `sizes.json` using the new `measure.mjs` script.
Plain rebuilds remove stale compressed counterparts.

The npm workspace adds `workbench-wasm` and `embed-workbench-wasm`. Frontend
assets are generated under `cmd/satpulsewbwasm/dist`, which is ignored by Git;
they are separate static files, not embedded inside the wasm module.
The Go shell has a `go:generate` directive in a `generate`-tagged file, so
frontend regeneration works on ordinary hosts. Native
Workbench assets were also regenerated for the shared UI changes.

The new Playwright project in `webui/packages/e2e/workbench-wasm` loads real
Go wasm and browser Web Streams. `webui/packages/e2e/wasmsim` runs the existing
u-blox simulator on stdin/stdout. Only the Web Serial device API is mocked.
The tests cover probing/configuration/readback, message sending, packet logging
and decoding, baud changes, repeated reconnects, serial fault recovery,
concurrent opens, cancellation and stream cleanup, and visible runtime failure.
They do not validate physical USB or permission-prompt behavior.

The default e2e suite includes functional WASM tests. CI prepares their assets
and simulator with `make -f Makefile.wasm test-assets`, and executes shell Go
tests under Node with `test-go`. The js/wasm compile workflow covers the GPS
packages and the new shell. Mock-transport UI tests and serial/runtime unit
tests run in the normal frontend suite without loading WASM.

Cold compressed startup measurements are separate from functional tests:
`make -f Makefile.wasm benchmark` selects the benchmark configuration, including
its compression and network throttling.

Initial prototype measurements were taken after Go, frontend and browser
validation. The full module measured 13,496,595 bytes raw,
3,205,339 bytes gzip, and 2,302,058 bytes Brotli. Cold startup measured about
0.29 s locally, 3.01 s at 10 Mbps, and 9.78 s at 2 Mbps with Brotli on desktop
Chromium; throttled runs used 50 ms latency. These are initial prototype
measurements; remeasure startup and size for release builds.

## Deployment

Plan to publish the browser app at `https://app.satpulse.net/` from a separate
GitHub Pages deployment repository. Application source stays in `satpulse`;
the deployment repository holds the workflow, version configuration, and a
README linking to the source. Build the wasm in Actions and deploy generated
files as a Pages artifact, keeping wasm binaries out of Git history. This
deployment has not been implemented. GitHub supports this through
[custom Pages workflows](https://docs.github.com/en/pages/getting-started-with-github-pages/using-custom-workflows-with-github-pages).

The site can host several versions under separate directories, for example:

```text
https://app.satpulse.net/0.3.0/
https://app.satpulse.net/0.4.0-beta1/
```

Each directory contains its own HTML, JavaScript, CSS, `wasm_exec.js`, and
`workbench.wasm`. The existing frontend uses relative asset URLs, so it can
load from a version directory. Always copy `wasm_exec.js` from the same Go
toolchain that builds that version's wasm. Version-specific directories also
keep assets from different releases apart in browser caches. The root may
list available versions or redirect to a selected default; that choice remains
open.

Deployment configuration should map each published version to an immutable
SatPulse source commit, recording the release tag and build toolchain versions.
Use that checkout's frontend dependency lockfile. App releases are selected
explicitly, independently of documentation deployments from `master`. Treat
published version URLs as fixed builds; changes to a released build need a new
build identifier rather than silently replacing its files.

Each deployment should assemble the complete site, including every retained
version. Decide whether to rebuild those versions from pinned inputs or restore
their outputs from persistent storage such as GitHub release assets. Ordinary
Actions artifacts expire and should not be the only copy needed to retain an
older version. The browser fetches files from Pages, not artifact download
links. See [artifact retention](https://docs.github.com/en/actions/how-tos/manage-workflow-runs/download-workflow-artifacts).

Configure the deployment repository's Pages custom domain as `app.satpulse.net`,
add a DNS CNAME pointing to `jclark.github.io`, and enable HTTPS enforcement
after certificate provisioning. See [custom subdomain setup](https://docs.github.com/en/pages/configuring-a-custom-domain-for-your-github-pages-site/managing-a-custom-domain-for-your-github-pages-site#configuring-a-subdomain).

Before publishing, verify startup from a version directory, including
`?vendor=Zhongke`, the wasm response's `application/wasm` content type, and
actual transfer compression. Generated `.gz` or `.br` files alone do not
establish how Pages will serve them. Account for the size of all retained
versions against the published-site limit of 1 GB and the soft bandwidth limit
of 100 GB/month. See [Pages limits](https://docs.github.com/en/pages/getting-started-with-github-pages/github-pages-limits).

## Prerequisite branches

This branch incorporates `non-unix` (#483), `session-conn` (#484), and
`wb-corrections-transport` (#485), each developed independently from `master`
and merged here. The first two provide platform support and explicit serial
connection contracts; the third makes correction forwarding an optional
frontend capability. These are the foundation PRs for #486.

## Commit strategy

The implementation has three independently reviewable commit boundaries:

1. **Plan.** This document, including deployment design and future work.
2. **Shared Workbench changes.** The common connection snapshot type and
   message-file preselection helper, native shell adoption, focused Go tests,
   optional device picker, independent mock-transport tests, and all regenerated
   `cmd/satpulsewb/dist` assets. Correction capabilities and their tests belong
   to the prerequisite refactor.
3. **Browser implementation.** The Go wasm shell and serial adapter, JavaScript
   transport and entry package, build/workspace/CI configuration, unit and
   browser integration tests, simulator, and related documentation and notes.

Generated browser frontend assets and WASM binaries remain ignored build output.

## Future work: Direct Ntrip v2 corrections

Investigate a direct browser client for the HTTP transport in Ntrip v2, using
an HTTPS caster endpoint and streaming `fetch`. The browser would send Basic
authentication directly to the caster and feed correction bytes into the Go
correction pipeline. The credentials would not pass through our hosting
server. This approach has not been implemented or tested.

The caster must allow our Workbench origin with CORS, successfully handle
unauthenticated `OPTIONS` preflights, and explicitly permit request headers
such as `Authorization` and `Ntrip-Version`. The actual correction response
also needs `Access-Control-Allow-Origin`. These permissions must come from
the caster; changing the Workbench hosting server's headers cannot grant them
for another server. Verify browser-managed request headers against the target
caster as part of the experiment.

Start with fixed-base streams. Separately investigate initial position through
`Ntrip-GGA` and the needs of VRS mountpoints: browser `fetch` cannot write
periodic GGA bytes back over an ongoing GET connection. CORS alone does not
resolve that part of the protocol.

Future integration would need a browser correction source and a way for
`Session.StartCorrections` to use it: that method currently constructs native
TCP/Ntrip sources directly. Source configuration must also identify the HTTPS
endpoint. These are prospective API changes, not part of the connection-contract
changes already implemented. Bind correction start/stop into the wasm shell
and enable the Corrections panel once the browser transport is usable.

Validate streaming delivery to the simulated receiver, cancellation, failed
authentication, and both permitted and rejected CORS requests before testing
a real caster. An ordinary relay that performs the Ntrip login would receive
users' caster credentials, so it is not the default approach for this future
work.

References: [BKG Ntrip overview](https://igs.bkg.bund.de/ntrip/),
[Fetch CORS protocol](https://fetch.spec.whatwg.org/#http-cors-protocol),
[Fetch request restrictions](https://fetch.spec.whatwg.org/#dom-request).

## Open questions

1. Physical receiver behavior across permission grants, baud close/reopen,
   DTR changes, resets, and USB re-enumeration.
2. Whether startup and memory use remain acceptable on slower client devices.
3. Which Ntrip v2 casters can support direct HTTPS/CORS access, and what subset
   of position-upload behavior is practical through browser HTTP APIs.
