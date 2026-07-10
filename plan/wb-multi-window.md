# Multi-window workbench

Plan for supporting multiple browser windows on one satpulsewb
instance, in two parts. The first part, read-only windows, lands
after the message-file PR. It keeps exactly one window able to
change state - a delta on the single-seat restriction
([archive/wb-single-seat.md](archive/wb-single-seat.md)) - while
every other window becomes a live, read-only view. The second part
is the full multi-window design: any number of windows, each a
complete workbench, worked through tab by tab. Read-only windows
may prove to be the right end state for a commissioning tool; the
full design is kept both as the reference for lifting the last
restriction and because its per-tab analysis supplies the
broadcast reworks part one depends on.

## Background

The workbench frontend was written for the Wails desktop app, where
the runtime guarantees exactly one window. Every panel could assume
it was the sole observer and sole driver of the session: if this
window didn't change something, nothing changed it. satpulsewb
serves the same frontend over HTTP to any number of windows.

Most of the app survives that transition untouched, because most
state is broadcast: there is one receiver session, all of its state
flows to every window as SSE events with sticky-cache priming, and
every window renders the same truth. Monitoring tabs are correct
under N windows by construction - and duplicated monitoring windows
are genuinely useful, since each window scrolls and expands
sections independently. The problems are confined to the tabs that
compose and perform operations: Messages, Configuration, and
Corrections. Each holds window-local state that feeds server
operations, and each inherited desktop assumptions about being the
only observer.

Concepts that emerged from the message-files analysis and should be
tested against the other tabs before being codified as policy:

- Client state divides into four kinds: session state (shared,
  broadcast, mutated only via events), cached session state
  (window-local copy of server data; refresh freely, write back
  only as deltas), draft state (window-local user input that only
  parametrizes a future operation; committed in full by that
  operation; stored by the server never), and view state (feeds no
  operation; free to diverge).
- Draft fields carry a dirty bit: born clean (tracking the cache or
  a server suggestion), dirtied only by user edit, cleaned only by
  successful commit, explicit discard, or explicit re-read.
  Incoming session state never overwrites a dirty field. Never
  auto-trigger a server operation from a broadcast state
  transition - the transition may have been caused by any window.
- Execution is always session state even when composition is draft:
  the session's operation state machine is a write lock on the
  receiver port (config read, config apply, message send are
  mutually exclusive peers, one at a time, from any window), and
  in-progress operations broadcast so other windows can disable
  and label their controls.

## Part 1: read-only windows

Every window carries a read-only bit; exactly one window at a time
may be non-read-only (the writer). A read-only window is a full,
live view of the session - monitor tabs, the running corrections
session, the current configuration, the writer's message-file
activity - but cannot change anything: no connect/disconnect, no
config apply or read, no send, no corrections start/stop.

The connection panel gets a "read-only" checkbox. The first window
claims the writer role at boot; every later window starts
read-only. Unticking the box takes the writer role over; the old
writer's checkbox re-ticks and its mutating controls grey out, but
it keeps streaming - it degrades into a monitor, not a corpse.
This also improves the stale-tab safety story: a forgotten tab that
holds the writer role is dethroned the moment you untick elsewhere.

### Relation to the single-seat PR

The seat mechanism survives with its meaning changed from
"permission to exist" to "permission to write". The random seat
value and the 410-on-stale-seat check survive as is; the rest of
the protocol changes as specified below. The substantive work in
part 1 is the writer-role protocol and the broadcast prerequisites;
the read-only rendering itself is wide but shallow (a `readOnly`
flag threaded into every mutating control's disabled computation,
alongside the existing `!connected` inputs).

### Writer-role protocol

- Not every POST is a write. Writer-gated POSTs (seat required,
  410 when stale): connect, disconnect, config/read, config/apply,
  corrections/start, corrections/stop, msgfile/select, msgfile/send,
  msgfile/cancel. Reader-safe POSTs (token + JSON only, no seat):
  signals (catalog lookup), decode-packet (used by the packet
  panel), and the five geo conversions (used by scatter/map/PVT
  views). The server needs two POST wrappers accordingly.
- `/sse` drops its seat requirement entirely: streams are reads,
  open to any token holder. The 204 stale-stream path and the
  claim-vs-subscribe atomicity go away.
- The claim endpoint gains modes:
  `POST /api/seat {"mode":"boot"|"take"|"release"}`. `boot`
  claims the writer role only if nobody holds it, returning
  `{"seat":"<value>","writer":"<grant>"}` when granted and
  `{"writer":"<current-grant>"}` when not (the window starts
  read-only). `take` always claims (the untick).
  `release` requires `seat=<value>` on the query and leaves the role
  unheld (the writer ticking its own box). A stale or missing release
  seat gets 410. `boot` and `take` do not require a seat. All modes
  remain token-checked and JSON-only.
- Every change of writer broadcasts a sticky SSE event (say
  `writer`, payload `{"writer":"<grant>","held":true}`; release
  broadcasts `{"writer":"","held":false}`. `writer` is an opaque
  random grant ID distinct from the secret seat; it is regenerated
  on every successful claim and safe to broadcast. A random grant
  rather than a process-local counter prevents a page from confusing
  a writer from a previous server run with the current writer. A
  successful claim returns both the seat and its grant ID; an
  unsuccessful boot claim returns the current public grant ID but no
  seat.

  A window is the writer iff the grant ID from its own successful
  claim equals the latest broadcast ID and it still holds the
  corresponding seat; on mismatch its checkbox re-ticks and controls
  grey. No stream is terminated and nothing targets the old writer
  specifically - the hub's takeover kill machinery is deleted; each
  window self-determines from the broadcast. A 410 on a writer-gated
  POST likewise clears the local seat and flips the window to read-only
  (belt and braces), replacing the terminal overlay and "Use here"
  button.
- The HTTP transport's seat becomes mutable state consulted per
  request (today it is captured into an immutable query string at
  boot), and the transport exposes claim/release through an optional
  writer-role capability plus the writer-state event to the app. A
  410 clears both the local seat and grant. Reader-safe POSTs use only
  the token query; writer POSTs append the current seat. The desktop
  transport has no writer-role capability and remains always-writer.
- Claim/release controls stay pending until the HTTP result and the
  authoritative sticky writer event agree. In particular, an HTTP
  response from a claim that has already lost to a later grant must
  not briefly enable writer controls.
- Simultaneous unticks resolve as last-claim-wins; the latest sticky
  grant is authoritative even if an earlier claimant's HTTP response
  arrives later, so the loser's checkbox snaps back. A writer that
  loses the role mid-operation is harmless: the in-flight
  operation completes on the server context; its next action gets
  410.
- There is no liveness tracking: a crashed or closed writer window
  leaves the role held. That is acceptable because taking the role
  is one click in any other window.

The app owns the resulting `readOnly` flag. It disables every control
that composes or starts a writer operation: connection fields and
connect/disconnect, configuration fields plus Read/Apply, corrections
source fields plus start/stop, and message-file selection, tag
composition, send, and cancel. Monitor navigation, packet inspection,
and other reader operations remain enabled. The server-side seat check
is still authoritative; disabled controls are the UX.

### Broadcast prerequisites

Read-only windows can only display what broadcasts, and two tabs
are currently fed by POSTs, so part 1 pulls these reworks forward
from the per-tab designs in part 2:

- Configuration: config props become a sticky broadcast event and
  the excursion-triggered readback machinery is deleted (a
  read-only window running today's trigger would POST a readback,
  get 410, and toast errors). This resolves the Configuration
  section's open decision 1 in favour of broadcasting. The server
  keeps a composite last-known-configuration snapshot: a successful
  Read replaces it wholesale; a successful Apply merges in the
  partial ConfigProps the run accumulated (ConfigProps records
  which properties are present, so the overlay is mechanical); an
  Apply carrying a reset invalidates it, since the receiver's
  configuration may then be arbitrarily different. What broadcasts
  as the sticky `gps:configprops` event is always the full
  composite, so sticky priming stays correct and an apply costs no
  extra receiver transaction. The snapshot is cleared on
  disconnect.

  The full readback happens when it is needed: the writer triggers
  one Read on first viewing the Configuration tab after acquiring
  the writer role (immediately, if already viewing it), and
  likewise when it views the tab with the composite empty or
  invalidated - which subsumes initial population (first writer,
  empty cache) and re-reading after a reset. The rationale: a
  window about to change the configuration deserves ground truth,
  while viewers tolerate the merged composite, whose only staleness
  risk is a set with side effects the run did not read back. The
  trigger keys to the window's own grant and its own tab view - a
  local action, not a broadcast transition, so the no-auto-trigger
  rule is respected. A viewer never auto-reads.
  Every window always caches the newest snapshot, but the
  form applies it only to clean groups. Group-level dirty bits remain
  the granularity for part 1. Clearing a dirty bit after a successful
  Apply adopts the already-cached snapshot, so event/HTTP-response
  ordering cannot lose the readback. The speed seeding effect is fixed
  to respect `speedTouched` at the same time.
- Messages: the loaded file (path + tags) and send activity become
  visible as sticky broadcast state so a read-only window can watch
  the writer's message work. Selecting a file broadcasts its path and
  tags. Sending maintains a shared activity snapshot containing the
  active tag, session, per-message send state, and accumulated
  responses; each update replaces the sticky snapshot, and loading a
  different file clears it. A viewer joining mid-send is therefore
  primed with the activity so far. Catalog choice, port, and save stay
  writer-local drafts. This inverts the part-2 decision to scope
  send/response display to the initiating window: with one writer,
  viewers watching the writer's sends is the natural model.
- Corrections: the running-source display fix already planned
  (show mode/host/mountpoint from the CorrEvent), plus the stats
  reset rekeyed to broadcast session transitions - a viewer whose
  CorMsgPanel resets only on its own Connect click would mix
  statistics across the writer's correction sessions.

### Sequencing

Part 1 sits after `wb-msgfile` in the stack, not directly on
`wb-single-seat`: its Messages prerequisite has nothing to attach
to before the HTTP message-file capability exists. If earlier
delivery matters, the broadcast reworks (config props, corrections
display) could land first while the terminal single-seat UI is
retained, with the checkbox model arriving once all viewer state
is available.

### Testing

Server tests cover boot claim with a free and occupied role, forced
claim, release, stale release and writer POSTs, reader POSTs without a
seat, last-claim-wins, seat-free SSE, and sticky writer-role priming.
The wb smoke helpers gain claim modes and reader/writer POST checks.
Manual two-window testing verifies first-writer/later-viewer boot,
control disabling, takeover without stream termination, release to
an all-read-only state, and recovery after a missed role event via
410. Once the phase-8 Playwright harness is in the branch ancestry,
encode the same two-page journey there, including config snapshot
priming without viewer readbacks, shared loaded-file and mid-send
activity, the running corrections source, and correction-statistics
reset between sessions.

### What this collapses in the full design

With exactly one writer there are no concurrent edits (same-group
conflict policy: moot), no cross-window operation collisions (reqID
scoping: unnecessary), no cross-window send or corrections
preemption, and the message-file version map loses its main driver:
a single writer with a single pin is coherent again, needing only
the loaded-file broadcast above. The version map's
loading-consistency machinery (mtime/include-closure change
detection) remains valuable for the edit-on-disk workflow but is no
longer fixing a race. If read-only windows prove sufficient as the
end state, part 2 remains the record of what full multi-writer
would take.

## Part 2: full multi-window, tab by tab

Any number of windows, each a full workbench with write capability,
coherence coming from the state taxonomy rather than a writer role.
Worked through one tab at a time; the general policy is synthesized
at the end. These designs assume multiple concurrent writers; where
part 1 changes a decision (send/response scoping), the part-1 text
notes it.

### Message files

Design for the Messages tab, worked out in full. The types and
semantics here are implementable as specified.

#### Problem

The desktop design keeps ONE parsed message file in the session
(`Session.msgFile`, set by `SetMsgFile`), and the send request
identifies only a tag within it. With one window that is a pin: the
send is guaranteed to draw on the same loading of the file whose
tags the user is looking at. With N windows it is a trap: window
B's select silently retargets window A's send (the review finding
on PR #366). The single implicit pin must become N explicit ones.

There is a second, related requirement: loading a message file at
different times can give different results. The file may be edited
on disk (or a file it includes may be), deleted, or shadowed by a
new file earlier on the search path. Operations that follow from a
display - above all, sending a tag to the receiver - must bind to
the particular loading that produced the display, never to "the
file as it happens to be now". The user must never send bytes he
has not inspected.

#### Design summary

The session maintains, per message-file name, a history of
immutable versions of that file as the session has observed it. A
version is created by loading; it never changes and is never
removed for the lifetime of the process. Shells (the HTTP server,
the desktop shell later) refer to a particular loading as (Name,
generation), where generation is the index into the history. Every
operation that applies to a loaded file - fetching its tags,
sending a tag - carries (Name, generation). There is no server-side
notion of a currently selected file; `Session.msgFile` and
`SetMsgFile` are deleted.

Two windows that load the same unchanged file share a generation.
A window pinned to an old generation keeps working after another
window loads a newer one, and after the file is deleted from disk.

#### The version map

New file `gps/app/session/msgfile.go` in package session. Types are
unexported: nothing outside the package sees them - shells see only
`msgfile.Name`, a generation int, and `[]MsgFileTag`.

    // msgFileVersion is one immutable loading of a message file.
    type msgFileVersion struct {
        parsed  *msgfile.Parsed
        path    string       // resolved path ("" for registered content)
        stamps  []fileStamp  // include closure; nil for registered content
        tags    []MsgFileTag // projection, computed once at creation
    }

    // fileStamp records a file's identity at load time.
    type fileStamp struct {
        path  string
        mtime time.Time
    }

    // msgFileHistory is everything the session knows about one name.
    type msgFileHistory struct {
        present  bool // found by the most recent catalog scan
        versions []*msgFileVersion // index = generation
    }

The Session gains `msgFiles map[msgfile.Name]*msgFileHistory`
guarded by its own mutex (operations on the map do not touch the
connection state machine), and the search path `msgFileDirs
[]string` supplied through the session Options by the shell
(satpulsewb passes its `msgDirs()`; the desktop shell will pass
nil - it has no library).

The map only grows. Files are KB-scale and this is a
commissioning-tool process; nothing is evicted. Across a server
restart all generations are gone, and a stale (Name, generation)
reference gets an error telling the user to reload.

#### Loading and change detection

    func (s *Session) LoadMsgFile(name msgfile.Name) (gen int, path string, tags []MsgFileTag, err error)

Resolve the name with `msgfile.FindName(name, s.msgFileDirs)`. If
the history's latest version has the same resolved path and an
unchanged closure (same file set, all mtimes equal), return that
generation - this is what lets two windows share a loading. If
anything differs (any mtime, closure membership, or the resolved
path itself - a new file in an earlier search dir shadows the old
one), load fresh and append a new version.

The change check MUST cover the whole include closure: editing an
included file does not touch the top file's mtime. `msgfile.Load`
already walks the include tree collecting the absolute path of
every file it reads (`loadTree`/`processFile`); expose that list
(e.g. a `Files []string` field on `Parsed`, top file first) and
stat each entry after a successful load to build the stamps.

Concurrent loads of the same name are resolved by re-checking under
the map lock before appending: parse outside the lock, and if an
identical version (same path and stamps) was appended meanwhile,
return its generation instead of appending a duplicate.

The tag projection (`TagDescs` + `msgTagCaps`, the body of today's
`SetMsgFile` minus the store) runs once at version creation and is
stored in the version.

#### Sending

    func (s *Session) SendMsgFile(name msgfile.Name, gen int, tag, port string, save bool, reqID string) error

Resolve (name, gen) in the map - unknown name or generation is an
error ("unknown message file version; reload the file") - then
proceed exactly as today's `SendMsgFile` body from `TaggedMsgs`
onward, using the version's `parsed`. The state-machine gate is
unchanged: sends and config operations remain mutually exclusive
(the port write lock). Two error-message fixes while touching the
gates, in `SendMsgFile`, `ReadConfig`, and `ApplyConfig`: when the
state is `StateConfiguring` or `StateSending`, say "another
operation is in progress"; keep "not connected" for the genuinely
disconnected states.

`reqID` is a client-generated opaque string identifying this send
for event routing (below). It is threaded through to the emitted
events; the internal `respSession` counter stays as is.

#### Catalog

    func (s *Session) MsgFileCatalog() []msgfile.Entry

Scan the search path (`msgfile.ListNames`), update every history's
`present` bit (true for names found, false for library names no
longer found), create empty histories for newly seen names, and
return the present entries. If the set of present entries changed
since the previous scan, emit a sticky event `gps:msgfiles` whose
payload is the entry list, so every window's picker updates when
any window triggers a scan. Scans are triggered by catalog requests
(the frontend already refetches on tab-visible); there is no
watcher and no timer.

Deleting a file from disk therefore removes it from the picker
everywhere, but its history and versions survive: a window pinned
to a loading of a deleted file keeps its tag table and can still
send from it. That is correct pin semantics - the window's display
is still true of that loading.

#### Registered entries (design only)

Two future constructors create versions without the library: the
desktop shell registering a dialog-chosen path (phase 4 of the
satpulseweb plan), and the workbench ad-hoc editor registering
pasted TOML content (deferred). Both produce entries under
`Name{Vendor: "", File: <generated>}`. The empty vendor cannot
collide with library names (`FindName` component validation never
produces it). Registered entries are unlisted: no scan ever sets
their `present` bit, so they never appear in any catalog; they are
reachable only by (Name, generation), which only the registering
window knows. Pasted content is parsed by a from-bytes variant of
`Load` that rejects `[[include]]` entries with an error; path
registration keeps the stamp machinery. Nothing in the message-file
change depends on these; the map design just accommodates them.

#### Send/response event routing

Today every window latches `respSessionRef` from the broadcast
`gps:msgsend` started event, so every window's Messages console
shows whichever send is in flight, and the latch is cleared as a
side effect of `configuring`/`disconnected` state transitions
(app.tsx). Decision (under multiple writers; part 1 inverts this):
responses display only in the window that initiated the send.

Mechanism: `MsgSendEvent` and `ResponseEvent` gain a `ReqID string`
json field. The window generates a fresh reqID per send (any
collision-resistant string; a random prefix plus counter is fine),
passes it in the send request, and filters both event streams to
its own reqID. The `respSessionRef` latch and its
clear-on-configuring hack are removed; "clear the console" (on
loading a file or clicking a tag) becomes forgetting the window's
own current reqID.

The broadcast still carries every send's events, which gives a
blocking status line for free: when a send is in flight (started
seen, no terminal event yet) whose reqID is not the window's own,
the Messages tab shows "sending from another window" next to the
disabled Send button. Send buttons in other windows are already
disabled while the broadcast state is `sending`; this line says
why. The receive tail keeps its current semantics: after the last
message is sent the worker listens for response stragglers until a
deadline, the state returns to `connected`, and the next operation
(from any window) may cut the tail short - preemption of receiving
only, accepted.

#### HTTP API (cmd/satpulsewb)

- `GET /api/msgfile/catalog` -> `{names: []Entry, preselect}`.
  Unchanged shape; now calls `Session.MsgFileCatalog` (triggering
  the scan/broadcast). Preselect logic stays in the server.
- `POST /api/msgfile/load {vendor, file}` -> `{gen, path, tags}`.
  Renamed from `/api/msgfile/select` - nothing is selected
  server-side any more; the verb matches the UI button. Calls
  `LoadMsgFile`; FindName/parse errors are 400 as today.
- `POST /api/msgfile/send {vendor, file, gen, tag, port, save,
  reqID}` -> `{}`. Unknown (name, gen) is 400 with the reload
  message.
- `POST /api/msgfile/cancel` unchanged (still unused by the UI).

The satpulsewb sink registers `gps:msgfiles` as a sticky event so a
newly connected window primes with the current catalog.

#### Frontend (webui/packages/workbench)

- `transport.ts`: `MsgFileInfo` becomes `{vendor, file, gen, path,
  tags}`; `selectMsgFile(vendor, file)` keeps its name but returns
  the new shape; `sendMsgFile` takes `(sel: {vendor, file, gen},
  tag, port, save, reqID)`. The desktop transport will implement
  the same shapes via registration.
- app state: the loaded file becomes `{vendor, file, gen, path,
  tags}` (extending today's `msgFilePath`/`msgFileTags`). The
  loaded-file display and the send must key off this pin, not off
  the picker dropdowns - the picker's keep-valid reset when a file
  leaves the catalog must not disturb the loaded file (the current
  code already separates these; preserve it).
- event handling: filter `gps:msgsend`/`gps:response` by own reqID;
  delete `respSessionRef` and its clearing at the
  `configuring`/`disconnected` transitions; track the foreign
  in-flight send for the status line.
- picker: subscribe to `gps:msgfiles` and rebuild the vendor/file
  lists from it; keep the visible-refetch (it is what triggers
  scans).
- Rebuild the embedded dist.

#### Testing

Session unit tests (new msgfile_test.go, table-driven, tmp dirs):
same-mtime reload dedups to the same generation; touching the top
file, touching an included file, and shadowing via an earlier
search dir each produce a new generation; a deleted file's catalog
omits it while send by the old (name, gen) still resolves; unknown
generation errors; the busy message distinguishes in-progress from
disconnected. satpulsewb handler tests updated for the new
load/send shapes and the sticky catalog event. `make test`, `go
test -race` on session and satpulsewb, both npm typechecks, and the
wb smoke scenarios must pass; verify by curl against a live replay
(catalog scan broadcast, load returning gen, send with gen, stale
gen after edit-and-reload).

### Configuration

Problems worked through; design direction identified; open
decisions listed at the end for discussion.

#### State inventory

Window-local, in app.tsx (survives tab switches):

- `configProps` - the last readback snapshot. Cached session state.
  Also feeds cross-tab consumers: `defaultPort` (the Messages tab's
  port seed) is derived from `configProps.port`.
- `selectedSignals` / `signalCatalog` - the signal selection
  (draft, edited via the signal picker) and the receiver's signal
  catalog (cache).
- `speed` - from the broadcast `gps:speed` event. Session state.

Window-local, in config-panel.tsx:

- Form fields per group: time mode (mobile/survey/fixed + survey
  and fixed-position parameters), time pulse (period, width, align,
  locked, polarity, GNSS, cable delay), signals (+ min elevation),
  message groups (NMEA/RTCM/PVT/Sats/Raw wire flags), serial speed.
  Draft state.
- Per-group dirty bits: `timePulseTouched`, `timeModeTouched`,
  `signalsTouched`, `speedTouched`, and the message groups'
  `*Change` flags. Granularity is the group, not the field.
- One-shot operations, not state: `saveType`, `resetType`, and the
  survey trigger parameters. Cleared after apply.
- `hasReadback` ref + `reading`/`applying` flags - readback trigger
  and in-flight markers.
- Client-side field validation, `readbackStationary`,
  `baudRateApplicable` - view state derived from the form/cache.

Server-side: nothing is stored. `ReadConfig`/`ApplyConfig` are
receiver transactions serialized by the session state machine (the
port write lock); `ApplyConfig` discards the `ConfigProps` the
configure run accumulates and returns only an error.

#### What breaks under N windows

- The readback trigger treats any excursion through the broadcast
  `configuring` state as a disconnect (`hasReadback` reset) and
  re-reads on the next visible+connected. Under two windows this
  wipes pending edits when any window reads or applies
  (`populateFromConfig` + clearing every dirty bit), collides
  concurrent auto-readbacks (one window gets a spurious error
  toast), and appears to re-trigger readbacks between two windows
  indefinitely (timing dependent; code-implied, not yet observed).
- The readback result returns only to the requesting window, so
  other windows' caches (and their `defaultPort` for the Messages
  tab) go stale silently.
- The busy error is mislabeled "not connected" (shared fix with
  the message-files section).
- A dirty-bit audit item independent of window count: the speed
  seeding effect (`setSelectedSpeed(speed)` on every `gps:speed`
  event) overwrites the dropdown without consulting `speedTouched`,
  violating the dirty-bit rule whenever a speed event arrives while
  the user has an unapplied speed selection.

What already works: apply is delta-only (touched groups), so
disjoint edits from two windows compose; controls grey out in every
window during any window's operation (the broadcast state does
this); apply errors return to the initiator on its own request.

#### Design direction

The result of a config read or apply is session state and should be
broadcast as a sticky event (say `gps:configprops`): every window's
cache refreshes from it, with the dirty-bit rule protecting edits
(clean groups repopulate, dirty groups untouched). The committing
window repopulates through the same path as everyone else - it
learns the outcome from the broadcast, keeping one code path. The
excursion-triggered re-read machinery is deleted: refresh happens
on data arrival, never on state transitions; a fresh window primes
from the sticky event. Part 1 triggers a read only from the writer:
on its first view of the Configuration tab after acquiring the
writer role, or on viewing the tab with the composite empty or
invalidated; a viewer never does. The explicit Read button remains for forcing a
re-read from the receiver. This also fixes the Messages tab's port
seeding for windows that never visited the Configuration tab.

Verified so far: `gpscfg.Configure` returns a `Result` whose
`ConfigProps` the configurator accumulates during the run, and
`Session.ApplyConfig` currently discards it. For u-blox the
configurator reads current values while planning sets, so an apply
run's ConfigProps covers roughly the properties the run touched -
not the full readback set. Coverage is protocol-dependent.

#### Open decisions

1. Broadcast scope: broadcast both read and apply results as
   `gps:configprops` (full direction above), or keep per-request
   responses and only fix the trigger (reset `hasReadback` on real
   disconnect only). The minimal fix stops the edit-wiping and the
   loop but leaves other windows' caches stale until a manual Read.
   (Part 1 resolves this in favour of broadcasting: read-only
   windows have no other way to see the configuration.)
2. Apply-result coverage: since an apply's ConfigProps is partial
   (per protocol), either broadcast it as a merge into the cached
   snapshot (event consumers must treat it as a delta), or have the
   server follow every apply with a full readback on the server
   side and broadcast that (one extra receiver transaction per
   apply, but the event is always a complete snapshot). Part 1
   merges into a server-side composite snapshot and takes the full
   readback on demand instead: on a newly granted writer's first
   view of the Configuration tab, and after a reset (which
   invalidates the composite).
3. Dirty-bit granularity: protection is per group today (a refresh
   must skip a whole touched group). Keep group granularity, or
   move to per-field bits so a refresh can update untouched fields
   inside a group the user has partially edited. Part 1 keeps group
   granularity.
4. Same-group concurrent edits from two windows: accept
   last-writer-wins (single-user tool), or detect and warn (needs
   the cache to version its snapshots). Moot under part 1.
5. Operation labeling: reuse the message-files reqID pattern for
   config operations so other windows can show "configuring from
   another window", or settle for a generic label keyed off the
   broadcast state alone.
6. The speed-seeding dirty-bit violation: make the seed respect
   `speedTouched`, or accept it (speed events during an unapplied
   selection are rare). Part 1 fixes it to respect the dirty bit.

### Corrections

Problems worked through. The corrections tab turns out to be the
closest to multi-window-correct already: its commit is
self-contained and its state broadcasts. The issues are
presentation-level.

#### State inventory

- Source form (mode, host, port, mountpoint, username, password,
  nmeaSend): draft state, persisted per field in localStorage.
  The commit is already self-contained: `StartCorrections` carries
  the complete `CorrectionSource`; the server stores no draft.
  localStorage gives same-browser windows shared persisted values
  at mount, live divergence while editing (each window writes on
  every keystroke, last writer wins in storage, no live sync);
  different browsers diverge entirely. All acceptable for drafts.
- Corrections state: broadcast `gps:corrections` events plus a
  snapshot fetch on connect, with an event-sequence guard against
  the snapshot racing an event. Correct under N windows as is.
- The form locks while running (`fieldsDisabled` keyed to the
  broadcast running state) - correct in every window.
- The Connect/Disconnect toggle: `running` is shared, so every
  window offers Disconnect while corrections run. Any window can
  stop - reasonable for one user with several windows.
- `StartCorrections` preempts at the session level: it stops any
  existing correction session and starts the new one
  (stopCorrLocked before start). The UI normally makes this
  unreachable (the button is Disconnect while running), so it only
  surfaces when two windows race Connect from the stopped state:
  last click wins silently, the loser's session is stopped moments
  after starting.
- Correction message statistics (CorMsgPanel): window-local
  accumulation of the broadcast `gps:corrpacket` events, cleared by
  a window-local counter (`sessionSeq`) bumped only by this
  window's own Connect click. Under N windows, a window that did
  not initiate the new correction session keeps accumulating stats
  across session boundaries - its table silently mixes two
  sessions.
- Base ARP clearing (app.tsx) keys off broadcast corrections state
  transitions - already correct under N windows.
- `nmeaPos` from broadcast `gps:nmeaPosition` - correct.

#### What needs changing

- Running-source visibility: `CorrEvent` already carries the
  running source's mode/host/port/mountpoint, but the panel ignores
  those fields. A window in a different browser (or with different
  saved drafts) shows its own greyed-out form values while
  something else entirely is running - the display lies. Show the
  running source from the event (a status line such as
  "ntrip://host:port/mountpoint" next to the state dot, or
  repopulating the disabled form display from the event while
  running).
- Stats reset: key the CorMsgPanel clear to broadcast corrections
  session transitions (the `connecting` state of a new session)
  instead of the window-local `sessionSeq`, so every window's
  statistics describe the current correction session.

#### Open decisions

1. DECIDED: start replaces. Only one correction session can
   logically exist; `StartCorrections` keeps its stop-then-start
   semantics, so a start while a session is running or starting
   replaces the old source. The UI keeps its explicit-disconnect
   discipline (Disconnect while running); the session stays
   tolerant of a one-click source switch.
2. Running-source display form: status line vs repopulating the
   disabled form fields from the event (the status line avoids
   touching the draft fields; repopulation reads better but must
   restore the window's own drafts on stop, and edits-vs-display
   need care).
3. Whether corrections operations want the reqID/"from another
   window" labeling at all, or whether the running-source display
   makes it moot (the source line already says what is running;
   who started it may not matter).

### General policy (after the digs)

Once all three tabs are designed, synthesize the state taxonomy and
the rules (Background above) into their final form, and add
enforcement guidance to webui/packages/workbench/src/CLAUDE.md so
future panel work classifies its state and honours the rules.
