# Workbench single seat

A PR on a branch `wb-single-seat` off `satpulsewb`, inserted into
the stacked-PR series directly after satpulsewb (#363): the PR is
based on `satpulsewb`, and the branch is then merged up into
`wb-smoketest` and `wb-msgfile`.

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

This PR restores the desktop model instead of fixing the panels:
exactly one active workbench window, with takeover by newest. A
newer window always wins; the dethroned window shows a terminal
screen. Takeover (rather than rejecting the newcomer) has no
stale-lock problem - a half-dead connection after laptop sleep
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
gates everything including the seat claim, and requireJSON still
guards every POST. The seat is a session-consistency mechanism, not
an auth mechanism, though it incidentally hardens CSRF further
(a cross-site attacker does not know the seat value).

### Frontend

The workbench-http transport claims the seat at boot (after token
setup, before opening the event stream), remembers it, and appends
it to the /sse URLs and every POST.

Losing the seat surfaces in two ways, both handled: the `takeover`
event on the stream (the fast path), and a 410 on any later POST
(the belt-and-braces path if the event was missed, e.g. the machine
was asleep). Either one puts the app into a terminal takeover
state: a full-screen overlay - "This workbench is now open in
another window" - with a single "Use here" button that reloads the
page, which re-claims the seat and dethrones the other window in
turn. No other UI is reachable from this state.

The workbench package learns about the takeover through a synthetic
transport event (the http transport emits it via the existing
eventsOn mechanism, e.g. `wb:seatlost`); the desktop transport
never emits it, so the desktop app is untouched.

## Implementation notes

- Server: seat value and claim/check logic in cmd/satpulsewb
  (server.go or a small seat.go); the sseHub gains "disconnect
  clients of old seats, sending a final takeover event". No
  gps/app/session changes.
- The post wrapper grows the seat check (after auth, before
  requireJSON); the sse handler validates the seat before
  subscribing.
- Frontend: seat claim + URL plumbing and 410 handling in
  webui/packages/workbench-http; the takeover overlay in
  webui/packages/workbench (reuse the styling approach of the
  existing stale-token auth notice). Rebuild the embedded dist.

## Stack integration

The wb smoke tests live on `wb-smoketest`, which follows this PR in
the stack, so this PR cannot run them. When merging `wb-single-seat`
up into `wb-smoketest`, the wb helpers must adapt: the scenario
Context claims a seat once (during wait_ready, from the printed
URL/token) and wb_post/wb_sse append it; checks that open several
streams share the Context's seat. Add a takeover check to
http/wb-default: claiming a second seat closes the first event
stream with a `takeover` event, and a POST with the old seat gets
410. The merge into `wb-msgfile` should be quiet.

## Testing

cmd/satpulsewb server tests: claim returns a seat and a second
claim invalidates the first (old stream receives takeover and
closes; old-seat POST gets 410; old-seat /sse gets 204); current
seat passes POSTs and streams; missing seat on /sse is 400; the
seat claim itself requires the token and JSON content-type. Both
npm typechecks; `make test`. Manual: two browser windows - second
takes over, first shows the overlay, "Use here" takes it back;
`curl` smoke of claim/410/204.

## Documentation

- Man page: a sentence under the URL/token description - opening
  the URL in a second window takes over the session; the first
  window is disconnected.
- docs/internals.md cmd/satpulsewb entry: mention the single-seat
  model.
- plan/satpulseweb.md Delivery section: record the inserted PR.
- No NEWS entry: the existing satpulsewb entry covers this
  (unreleased feature).
