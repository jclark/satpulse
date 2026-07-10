# Workbench single seat

Implemented after satpulsewb (#363) as part of the stacked-PR series.

## Background

The workbench frontend was written for the Wails desktop app, where
the runtime guarantees exactly one window, and every panel assumes
it is the sole observer and sole driver of the session. satpulsewb
serves the same frontend over HTTP, which has no one-window
guarantee: the user opens the printed URL twice, or a stale tab
from yesterday is still connected. Two windows then interfere - the
Configuration panel's readback trigger and pending edits, and the
Messages panel's select/send pairing (the review finding on PR
#366), both break under a second observer.

The implementation restores the desktop model instead of fixing the
panels: exactly one active workbench window, with each new seat claim
taking over from the previous one. The dethroned window shows a
terminal screen. Takeover (rather than rejecting the newcomer) has
no stale-lock problem - a half-dead connection after laptop sleep
cannot lock the user out - and doubles as the recovery path for a
lost window. Every panel's single-observer assumptions become valid
again with zero panel changes.

Supporting multiple windows properly is a separate, later effort
worked out tab by tab in [wb-multi-window.md](../wb-multi-window.md);
this restriction is the reliable interim model, and nothing here is
thrown away by that work (the seat either becomes the controller
seat of a controller/viewer model, or is removed).

## Design

### The seat

The server holds one seat, identified by an opaque random value
regenerated on every claim (128-bit hex from crypto/rand). A random
value rather than a counter means a seat from a previous server run
can never be mistaken for current.

- `POST /api/seat` (token-checked, JSON content-type like every
  other POST, empty JSON body) claims the seat: generate a new
  value, disconnect all SSE clients attached to the old seat
  (sending each a final `takeover` event before closing), and
  return `{"seat": "<value>"}`. This is the only POST that does not
  itself require a seat.
- `GET /sse?seat=<value>[&stream=packets]`: the seat must be
  current. A stale seat gets 204 No Content - the HTML spec's
  signal telling EventSource to stop reconnecting - so a dethroned
  window's auto-reconnect dies quietly. A missing seat parameter is
  400 (a client bug, not a lifecycle event). Multiple concurrent
  streams on the current seat are allowed: the events and packets
  streams share one seat, and a reconnect may briefly overlap the
  connection it replaces.
- All other POSTs require `seat=<value>` as a query parameter
  (alongside the existing `t` token parameter) matching the current
  seat. A stale or missing seat gets 410 Gone - distinct from the
  409 used for session-state conflicts, so the client can
  distinguish "you lost the seat" from "operation refused".
- GET endpoints stay seat-free: they are read-only, and keeping
  them open preserves easy curl debugging.

The existing auth and CSRF layers are unchanged: the token still
gates every API and SSE endpoint including the seat claim, and
requireJSON still guards every POST. The seat is a
session-consistency mechanism, not an auth mechanism, though it
incidentally hardens CSRF further (a cross-site attacker does not
know the seat value).

### Frontend

The workbench-http transport claims the seat at boot (after token
setup, before opening the event stream), remembers it, and appends
it to the /sse URLs and every POST.

Losing the seat surfaces through a `takeover` event on the stream, a
410 on a later POST, or an EventSource that closes terminally because
its seat became stale before the stream opened. In the last case the
transport probes a seat-free GET to distinguish a stale seat from
stale authentication after a server restart. Seat loss puts the app
into a terminal takeover state: a full-screen overlay - "This
workbench is now open in another window" - with a single "Use here"
button that reloads the page, re-claims the seat, and dethrones the
other window in turn. Stale authentication instead transitions the
app straight to a notice directing the user to the newly printed
URL; that transition never reloads and never reads the shared token
store, so a stale tab cannot reclaim a seat another tab has just
taken with a fresh token.

The workbench package learns about the takeover through a synthetic
transport event (the http transport emits it via the existing
eventsOn mechanism, e.g. `wb:seatlost`); the desktop transport
never emits it, so the desktop app is untouched.

## Implementation

- The seat value and claim/check logic are in `cmd/satpulsewb/server.go`;
  the SSE hub sends the final takeover event to clients of the old
  seat. No `gps/app/session` changes were needed.
- The POST wrapper checks the seat after auth and before requireJSON;
  the SSE handler validates the seat before subscribing.
- Seat claiming, URL plumbing, 410 handling, and terminal-close
  recovery are in `webui/packages/workbench-http`; the takeover
  overlay is in `webui/packages/workbench`.

## Testing

The `cmd/satpulsewb` server tests cover seat claims and replacement,
the takeover event, stale and missing seats on POST and SSE, current
seat access, and the token and JSON requirements on the claim.

## Documentation

The man page and `docs/internals.md` describe the single-seat model,
and `plan/satpulseweb.md` records the work in the delivery stack. No
NEWS entry was added because the existing unreleased satpulsewb entry
already covers it.
