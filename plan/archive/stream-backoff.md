# Adaptive backoff for stream

Replace the current simple exponential backoff in Pull's reader
goroutine with an adaptive backoff.  Encapsulate it in an unexported
`backoff` type in its own file (`backoff.go`) so Push can reuse it.

Package: `gps/app/stream`.

## Prerequisite

- `plan/corrsink-rename.md` (rename corrsink to stream).

## Reconnection with adaptive backoff

Both `stream.pull` and `stream.push` maintain persistent connections
with automatic reconnection.  The backoff adapts to connection
health.

**On disconnect:**

1. Wait for random duration in [0, current_backoff).
2. Attempt to reconnect.
3. On failure: multiply backoff by 1.5 (capped at 1 hour).
4. Go to step 1.

**On successful connect:**

1. Divide backoff by 1.5 (floor at 1 second).

**While connected:**

1. Every minute the connection stays up: divide backoff by 1.5
   (floor at 1 second).

**Parameters:**

- Initial backoff: 1 second.
- Multiplier: 1.5.
- Cap: 1 hour.
- Jitter: full jitter -- actual wait is uniform random in
  [0, current_backoff).
- Floor: 1 second.

The sequence of maximum waits on repeated failure: 1s, 1.5s, 2.2s,
3.4s, 5.1s, 7.6s, 11.4s, 17.1s, 25.6s, 38.4s, 57.7s, 1.4m, 2.2m,
3.2m, 4.9m, 7.3m, 10.9m, 16.4m, 24.6m, 36.9m, 55.4m, 1h.

A connection that stays up for N minutes reduces the backoff by
about N steps.  A connection that drops immediately after
reconnecting barely reduces the backoff, so flapping connections
escalate quickly.

## backoff type

```go
type backoff struct {
    cur float64    // current backoff in seconds
    rng *rand.Rand
}

func newBackoff() *backoff

// increase increases the backoff after a failed connection attempt.
func (b *backoff) increase()

// decrease reduces the backoff.  Called on successful connect and
// periodically while connected.
func (b *backoff) decrease()

// delay returns a random duration in [0, b.cur) to wait before
// the next connection attempt.
func (b *backoff) delay() time.Duration
```

Constants:

```go
const (
    backoffInitial    = 1.0       // seconds
    backoffMultiplier = 1.5
    backoffCap        = 3600.0    // 1 hour in seconds
    backoffFloor      = 1.0       // seconds
    backoffDecay      = time.Minute
)
```

`increase` multiplies cur by the multiplier (capped).  `decrease`
divides cur by the multiplier (floored).  `delay` returns a random
duration in [0, cur) using full jitter.

## Changes to Pull.reader

Replace the current `backoffWait` method and `backoff` local
variable with the new `backoff` type.  The reconnect loop becomes:

1. Call `source.Connect(ctx)`.
2. On error: `b.increase()`, wait for `b.delay()` using
   `select { case <-ctx.Done(): return; case <-time.After(d): }`,
   loop.
3. On success: `b.decrease()`.
4. Scan packets until read error or ctx cancel.  After each
   successful packet read, call `b.decrease()` if at least
   `backoffDecay` has elapsed since the last decrease (tracked
   by a simple `time.Since` check, no ticker needed).
5. On read error (not ctx cancel): `b.increase()`, wait as in
   step 2, loop.

The backoff is only accessed from the reader goroutine, so no
synchronization is needed.  The existing `errReconnect` sentinel
packet (which resets the pruning queue's MSM epoch tracking on
reconnect) must be preserved.

## Testing

Test the backoff type directly in `backoff_test.go`:

- Verify delay is in [0, cur).
- Verify increase multiplies cur by 1.5.
- Verify cap at 1 hour.
- Verify decrease divides cur by 1.5.
- Verify floor at 1 second.
