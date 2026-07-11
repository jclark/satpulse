# Workbench writer seat

Implemented in commit e8341e49 (PR #367). Superseded as a design
document by [wb-multi-window.md](wb-multi-window.md), which
records the multi-window model this change created and the
remaining work to make read-only windows fully live; this file
stays as the record of the change itself.

Replaces the single-seat design
([archive/wb-single-seat.md](archive/wb-single-seat.md), PR #367),
which was implemented but not merged.

## Background

The workbench frontend was written for the Wails desktop app, where
the runtime guarantees exactly one window. satpulsewb serves the
same frontend over HTTP to any number of windows: the user opens
the printed URL twice, or a stale tab from yesterday is still
connected.

Most of the app is already correct under N windows: there is one
receiver session, all of its state flows to every window as SSE
events with sticky-cache priming, and every window renders the same
truth. Duplicated monitoring windows are genuinely useful. The
multi-window analysis ([wb-multi-window.md](wb-multi-window.md))
established that every real interference between windows goes
through a mutating POST: window B's message-file select retargeting
window A's send, config applies and the Configuration panel's
automatic readback wiping another window's pending edits. Pure
observation never breaks anything.

The superseded design enforced a stronger invariant than needed -
one window rather than one writer - by terminating the dethroned
window's SSE streams. Termination was its only way to notify the
loser (the seat value is secret, so nothing was broadcastable) and
it cascaded: per-client kill machinery in the SSE hub, a 204
stale-seat path, claim-vs-subscribe atomicity, and an ambiguous
EventSource terminal close (stale seat vs stale token) needing a
probe request to disambiguate. This design broadcasts a public
grant identifier instead, and the whole chain disappears.

## Design

One invariant: at most one window holds the write seat; every other
window is a live, read-only viewer. No stream is ever terminated
and no window is ever killed.

### Seat and grant

The server holds one seat, represented by two values regenerated
together on every claim, both 128-bit hex from crypto/rand:

- the seat: secret, returned only to the claimant, carried on
  writer POSTs. Never broadcast - besides identifying the writer it
  hardens CSRF (a cross-site attacker does not know it), which
  matters when `-L` disables the token.
- the grant: public, broadcast to everyone, letting each window
  determine for itself whether it is the writer. Random rather than
  a counter so a grant from a previous server run can never be
  mistaken for current.

### Protocol

- `POST /api/seat` (token-checked, JSON content-type, empty body)
  always claims: generate a new seat and grant, return
  `{"seat":"...","grant":"..."}`, and broadcast the grant. Newest
  claim wins, unconditionally. There are no modes, no release, and
  no liveness tracking: every window claims at boot, so opening a
  fresh window - the natural act of a user who lost, closed, or
  reloaded the old one - always yields a working writer with zero
  friction. This is the only POST that does not itself require a
  seat.
- Every seat change broadcasts a sticky SSE event `writer` with
  payload `{"grant":"..."}`. Because it is sticky, a newly opened
  stream is primed with the current grant; a window that loses the
  race between claiming and subscribing simply learns the newer
  grant when its stream primes. No claim/subscribe atomicity is
  needed.
- Writer-gated POSTs - connect, disconnect, config/read,
  config/apply, corrections/start, corrections/stop, and the
  msgfile operations - require `seat=<value>` as a query parameter
  matching the current seat. Stale or missing gets 410 Gone,
  distinct from the 409 used for session-state conflicts.
- Reader POSTs - signals, decode-packet, and the five geo
  conversions - are reads that happen to be POSTs (used by monitor
  panels). They require only the token and JSON content-type. The
  server gets two POST wrappers accordingly.
- `GET /sse` and all other GETs are seat-free: streams and reads
  are open to any token holder, and curl debugging stays easy.

### Frontend

- The HTTP transport claims the seat at boot, before opening the
  event stream, retrying while the server is unreachable. It keeps
  the seat as mutable state and appends it to writer POSTs only.
- A window is the writer iff the latest broadcast grant equals the
  grant from its own claim. On mismatch it enters a read-only
  state; a 410 on a writer POST does the same (belt and braces for
  a missed event).
- Read-only rendering: the connection bar shows a persistent
  notice - "Read-only: this workbench has been taken over by
  another window" - with a "Use here" button that simply re-claims
  via `POST /api/seat`. No reload: the streams never closed, so the
  window keeps its scroll positions, expanded sections, and drafts,
  and becomes the writer in place. The dethroned window's monitor
  tabs, packet inspection, and geo lookups all stay live.
- Mutating controls disable through the same computations that
  already consult the connection state, driven by a read-only flag
  from the transport. One gate is load-bearing rather than
  cosmetic: the Configuration panel's automatic readback must fire
  only while this window holds the grant, so a read-only window
  doesn't POST readbacks in the background and collect 410 toasts.
- A read-only window's Configuration and Messages tabs are degraded
  (they show what the window last saw, or nothing). Making them
  live requires the broadcast reworks specified in
  wb-multi-window.md (config composite snapshot, message activity
  snapshot, corrections source display), which land later and
  independently; viewers exist for the monitor tabs meanwhile.
- Stale authentication stays cleanly separable: since SSE no longer
  checks the seat, a terminal EventSource close can only mean the
  token went stale after a server restart, and the app shows the
  existing notice pointing at the newly printed URL. The probe
  request and synthetic terminal events of the superseded design
  are gone.
- The desktop transport has no seat, is always the writer, and
  never emits the writer event; the desktop app is untouched.

### Relation to the multi-window plan

This is the protocol skeleton of the model wb-multi-window.md
describes: the seat/grant split, the sticky writer broadcast, the
writer/reader POST division, and 410-flips-to-read-only all carry
forward unchanged. What remains there is the broadcast reworks
that make viewer tabs fully live (and, only if the
takeover-on-open behaviour ever proves annoying in practice, a
read-only checkbox with claim modes).

## Implementation

Reworks the `wb-single-seat` branch in place. Kept from the
superseded implementation: the claim handler and requireSeat
wrapper in `cmd/satpulsewb/server.go`, the seat plumbing in the
HTTP transport and smoke tests, and most server tests. Deleted: the
SSE hub's takeover machinery (per-client seat, takeover channel,
kill loop), the 204 stale-seat path on `/sse` and its
claim-vs-subscribe locking, the transport's terminal-close probe
and synthetic terminal events, the full-screen takeover overlay,
and the smoke-test takeover choreography.

## Testing

Server tests: a claim returns seat and grant and broadcasts the
sticky writer event; a second claim supersedes the first (old seat
gets 410, new grant broadcasts); writer POSTs with stale or missing
seats get 410; reader POSTs succeed without a seat; SSE opens
without a seat and primes the writer grant. Smoke tests: checks
that write claim a seat and carry it on their writer POSTs; GETs
and SSE stay seat-free; a second claim delivers the new grant on a
still-open stream and 410s a POST from the superseded seat.
Manual two-window checks: takeover notice appears in the old
window, Use here takes back without reload, monitor tabs keep
updating throughout.

## Documentation

The man page and `docs/internals.md` describe the writer-seat
model; the existing unreleased satpulsewb NEWS entry already covers
it.
