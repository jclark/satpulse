# NTRIP server: `stream.push` to remote caster

Push RTCM data from the GPS receiver to a remote NTRIP caster
using the NTRIP server (SOURCE) protocol.  Implemented as a
`stream.Push` instance in `gps/app/stream`.

Related issues: #126 (NTRIP).

## Prerequisite

- `plan/corrsink-rename.md` (rename corrsink to stream).
- `plan/stream-backoff.md` (adaptive backoff).

## Configuration

```toml
[[stream.push]]
ntrip.address = "caster.example.com:2101"
ntrip.mountpoint = "MY_BASE"
ntrip.password = "secret"
```

`stream.push` is a table array (push to multiple destinations).
The configuration scheme can be extended later to push to a plain
TCP server by using `tcp.address` instead of `ntrip.*` keys.

NTRIP server authentication uses a password only (no username).
The server sends a `SOURCE <password> /<mountpoint>` request to
the caster.

### Options

- `protocol` -- restrict which packet formats are forwarded.
  Defaults to `"RTCM"` for NTRIP transport.
- `msm7to4` -- convert MSM7 packets to MSM4 before sending.

## Implementation

Lives in `gps/app/stream` alongside `Pull`.  The entry point type
is `Push`.  Reuses the `backoff` type from `plan/stream-backoff.md`
for reconnection and the `pruningQueue` for handling network delays.

`Push` subscribes to the existing packet bcast (the receiver's
scanned packets) and runs a three-goroutine pipeline:

1. **Reader** -- receives packets from the bcast subscription,
   filters to the relevant protocol.
2. **Pruning queue** -- deduplicates by message type, same as
   Pull's queue.
3. **Writer** -- maintains the NTRIP server connection to the
   remote caster (with adaptive backoff on reconnect) and writes
   packets to it.

Details still to be fleshed out.
