# Multi-window workbench

How satpulsewb supports multiple browser windows on one instance:
exactly one window at a time holds the write seat, and every
other window is a live, read-only viewer. Replaces the old
[wb-writer-seat.md](wb-writer-seat.md) plan. The seat protocol is
implemented (commit e8341e49, PR #367); the remaining work is
making viewer windows fully live on the tabs that are fed by
writer operations today. Lifting the one-writer restriction
entirely is a possible follow-on kept in
[wb-multi-window-write.md](wb-multi-window-write.md); read-only
viewers may well be the right end state for a commissioning tool.

## Background

The workbench frontend was written for the Wails desktop app,
where the runtime guarantees exactly one window. Every panel
could assume it was the sole observer and sole driver of the
session: if this window didn't change something, nothing changed
it. satpulsewb serves the same frontend over HTTP to any number
of windows.

Most of the app survives that transition untouched, because most
state is broadcast: there is one receiver session, all of its
state flows to every window as SSE events with sticky-cache
priming, and every window renders the same truth. Monitoring tabs
are correct under N windows by construction - and duplicated
monitoring windows are genuinely useful, since each window
scrolls and expands sections independently. The problems are
confined to the tabs that compose and perform operations:
Messages, Configuration, and Corrections. Each holds window-local
state that feeds server operations, and each inherited desktop
assumptions about being the only observer. Every real
interference between windows goes through a mutating POST, which
is why the writer seat restricts writes, not windows.

## The model as implemented

The server holds one write seat, identified by a random per-claim
value (128-bit hex; random rather than a counter so a value from
a previous server run can never be mistaken for current).
`POST /api/seat` always claims - newest window wins: it mints a
fresh value, returns it to the claimant, and broadcasts it to
every window as the sticky `writer` SSE event, so a fresh window
or a reload is always writable. Writer-gated POSTs (connect,
disconnect, config read/apply, corrections start/stop, and the
msgfile operations once wb-msgfile lands) carry the value and
answer 410 Gone when it is not the current one - a freshness
check, not authentication: every window that can claim is equally
trusted, so nothing about the value is secret. Reader POSTs
(signals, decode-packet, the geo conversions) and the SSE streams
carry only the token. A window is the writer iff the latest
broadcast value equals the one from its own claim; on mismatch
its mutating controls grey behind a connection-bar notice with a
"Use here" button that re-claims in place. No stream is ever
terminated. At startup the broadcast is seeded with a fresh
no-holder value, so a tab surviving a server restart learns
immediately that its old seat is stale.

There is no read-only mode a window can be put into or choose:
read-only is only ever the result of another window claiming
later. That keeps the protocol to one claim verb with no release
and no liveness tracking - a crashed or closed writer leaves
nothing locked, because taking the seat is one click (or just
opening a window). If deliberately opening a second window as a
monitor - which dethrones the window being driven - proves
annoying in practice, a claim-suppressing boot mode or read-only
checkbox could be added then; the protocol accommodates it.

## Remaining work: fully live viewers

A read-only window today shows live monitor tabs, the running
corrections session, and packet inspection, but the two tabs fed
by writer POSTs degrade: Configuration shows what the window last
read, Messages nothing. Three reworks make viewers whole. They
are independent of each other, and the seat protocol needs no
further change for any of them.

### Configuration: broadcast the snapshot

Config props become a sticky broadcast event (`gps:configprops`)
and the excursion-triggered readback machinery is deleted (a
read-only window running that trigger would POST a readback, get
410, and toast errors; until this rework it is gated on holding
the seat). The server keeps a composite last-known-configuration
snapshot: a successful Read replaces it wholesale; a successful
Apply merges in the partial ConfigProps the run accumulated
(ConfigProps records which properties are present, so the overlay
is mechanical); an Apply carrying a reset invalidates it, since
the receiver's configuration may then be arbitrarily different.
What broadcasts as the sticky event is always the full composite,
so sticky priming stays correct and an apply costs no extra
receiver transaction. The snapshot is cleared on disconnect.

The full readback happens when it is needed: the writer triggers
one Read on first viewing the Configuration tab after claiming
the seat (immediately, if already viewing it), and likewise when
it views the tab with the composite empty or invalidated - which
subsumes initial population and re-reading after a reset. The
rationale: a window about to change the configuration deserves
ground truth, while viewers tolerate the merged composite, whose
only staleness risk is a set with side effects the run did not
read back. The trigger keys to the window's own seat and its own
tab view - a local action, never a broadcast state transition,
which any window may have caused. A viewer never auto-reads. The
explicit Read button remains for forcing a re-read from the
receiver.

Every window always caches the newest snapshot, but the form
applies it only to clean groups; group-level dirty bits remain
the granularity. Clearing a dirty bit after a successful Apply
adopts the already-cached snapshot, so event/HTTP-response
ordering cannot lose the readback. The speed seeding effect is
fixed to respect `speedTouched` at the same time. This rework
also fixes the Messages tab's port seeding for windows that never
visited the Configuration tab: `defaultPort` derives from the
broadcast snapshot instead of a per-window readback result.

### Messages: broadcast the loaded file and send activity

The loaded file (path + tags) and send activity become visible as
sticky broadcast state so a read-only window can watch the
writer's message work. Selecting a file broadcasts its path and
tags. Sending maintains a shared activity snapshot containing the
active tag, session, per-message send state, and accumulated
responses; each update replaces the sticky snapshot, and loading
a different file clears it. A viewer joining mid-send is
therefore primed with the activity so far. Catalog choice, port,
and save stay writer-local drafts. With one writer, viewers
watching the writer's sends is the natural model.

Sequenced after wb-msgfile: there is nothing to attach to before
the HTTP message-file capability exists.

### Corrections: presentation fixes

`CorrEvent` already carries the running source's
mode/host/port/mountpoint, but the panel ignores those fields, so
a window with different saved drafts shows its own greyed-out
form values while something else entirely is running. Show the
running source from the event - a status line such as
"ntrip://host:port/mountpoint" next to the state dot, or
repopulating the disabled form display while running (the status
line avoids touching the draft fields). And the correction-
message statistics reset is rekeyed to broadcast session
transitions (the `connecting` state of a new session) instead of
the window-local Connect click, so a viewer's statistics describe
the current correction session rather than silently mixing
sessions.

## Testing

Manual two-window checks per rework: config snapshot priming in a
viewer without a readback of its own, dirty groups surviving the
writer's Read, shared loaded-file and mid-send activity, the
running corrections source, and statistics reset between
correction sessions. The wb smoke scenarios extend where the
server side changes (the configprops and message-activity sticky
events). Once the Playwright harness exists (phase 8 of the
satpulseweb plan), encode the two-window journeys there.
