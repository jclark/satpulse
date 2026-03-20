# NTRIP caster: `gps/app/ntrip`

Implements an NTRIP caster that serves RTCM correction data from the
GPS receiver to connecting NTRIP clients (rovers).  Addresses issue
#126 and issue #236 (MSM7→MSM4 conversion).

## Configuration

The caster is configured via `[ntrip]` and `[[ntrip.mountpoint]]`
tables in `satpulse.toml`.  Minimum configuration:

```toml
[[ntrip.mountpoint]]
path = "RTCM"
```

Full configuration:

```toml
[ntrip]
listen = ":2103"       # default ":2101"

[[ntrip.user]]
username = "rover1"
password = "secret"

[[ntrip.user]]
username = "rover2"
password = "secret2"

[[ntrip.mountpoint]]
path = "MSM7"
users = ["rover1"]     # restrict to these users; default: all

[[ntrip.mountpoint]]
path = "MSM4"
msm7to4 = true         # convert MSM7 to MSM4
users = ["rover1", "rover2"]
```

Rules:

- `listen` defaults to `":2101"` (IANA-assigned NTRIP port).
- At least one `[[ntrip.mountpoint]]` must be specified; otherwise
  the caster is not started.
- No `[[ntrip.user]]` entries means anonymous access for all
  mountpoints.
- `users` on a mountpoint restricts access to those users
  (requires corresponding `[[ntrip.user]]` entries).  If `users`
  is omitted or empty, all defined users (or anonymous) may access
  the mountpoint.
- `msm7to4 = true` converts MSM7 packets to MSM4 before sending.
  Non-MSM7 RTCM packets are forwarded unchanged.

## NTRIP protocol

We support both NTRIP v1 and v2 with auto-detection.  The server
uses Go's `net/http.Server` for request parsing, auth, and routing.
The response path differs by version.

**Version detection:**

The client's NTRIP version is determined by the `Ntrip-Version`
header.  If the request includes `Ntrip-Version: Ntrip/2.0`, the
client is treated as v2.  Otherwise, it is treated as v1.  This is
more reliable than checking `r.ProtoMinor`, since some v1 clients
send HTTP/1.1 requests.

**V1 response:**

The handler uses `http.Hijacker` to take over the raw connection
and writes `ICY 200 OK\r\n\r\n` followed by the raw RTCM byte
stream.

**V2 response:**

The handler uses `http.ResponseWriter` with
`http.NewResponseController.Flush()` to stream data.  Go
automatically applies `Transfer-Encoding: chunked` since
Content-Length is unknown.  This is correct per the NTRIP v2 spec.

HTTP/2 is disabled on this server (set `TLSNextProto` to empty map)
since NTRIP v2 means HTTP/1.1, not HTTP/2, and Go's HTTP/2
response writer does not support hijacking.

**Source table request** (`GET /`):

The caster responds with the NTRIP source table, listing each
configured mountpoint.  The response uses `Content-Type:
text/plain` and ends with `ENDSOURCETABLE`.  This is the same for
both v1 and v2.

**Stream request** (`GET /<mountpoint>`):

The handler checks basic auth (if users are configured), looks up
the mountpoint, detects the NTRIP version, and responds
accordingly.  It then streams raw RTCM data until the client
disconnects or the context is cancelled.

**Auth:**

HTTP basic auth via `r.BasicAuth()`.  If no `[[ntrip.user]]`
entries are defined, auth is not required.  If users are defined,
the request must include valid basic auth credentials.  The
mountpoint's `users` list further restricts which authenticated
users can access it.

## Architecture

The caster is a fan-out model: one source (the receiver's packet
bcast), many subscribers (connected clients).  Each client gets its
own goroutine that subscribes to the bcast, filters to RTCM packets,
optionally converts MSM7→MSM4, and writes to the TCP connection.

This is similar to `proxy.connWriteWorker` but with:

- RTCM-only filtering (using `scan.Packet.HasTag(gpsreg.TagRTCM)`)
- Optional MSM7→MSM4 conversion per mountpoint
- NTRIP HTTP framing on connection setup
- Auth and mountpoint routing

No pruning queue is needed: each client has its own TCP connection
with OS buffering.  If a client can't keep up, the bcast drops
messages for that subscriber (bcast's existing behavior) or the TCP
write blocks/errors and we disconnect the client.

## Package: `gps/app/ntrip`

Lives at the application layer in `gps/app/` alongside `bcast`,
`gpsio`, and `stream`.  It does logging and manages goroutines,
so it belongs at the app layer, not in `gps/lib/`.

### Dependencies

- `gps/app/bcast` -- subscribe to receiver's packet stream
- `gps/scan` -- `scan.Packet` type
- `gps/gpsreg` -- `TagRTCM` for filtering
- `gps/lib/rtcmbin` -- `ParseMsg`, `MSM7Convert`, `SerializeMsg`,
  `ExtractMsgType`, `MsgType.IsMSM`
- `gps/lib/ntriphdr` -- NTRIP version detection from request
  headers, `SOURCE` request parsing (for NTRIP server connections)
- `gps/gpsprot` -- `Tag` type

### Types

```go
// Config is the TOML configuration for the NTRIP caster.
type Config struct {
    Listen     string           `toml:"listen"`
    Users      []UserConfig     `toml:"user"`
    Mountpoint []MountConfig    `toml:"mountpoint"`
}

type UserConfig struct {
    Username string `toml:"username"`
    Password string `toml:"password"`
}

type MountConfig struct {
    Path    string   `toml:"path"`
    MSM7to4 bool    `toml:"msm7to4"`
    Users   []string `toml:"users"`
}
```

### Start function

```go
func Start(ctx context.Context, lg *slog.Logger,
    wg *sync.WaitGroup, cfg Config,
    b *bcast.Bcast[scan.Packet]) error
```

Follows the same pattern as `proxy.Start` and `startHTTP`:

1. Validate config (at least one mountpoint, users referenced in
   mountpoint `users` lists exist in `[[ntrip.user]]`, no duplicate
   paths, etc.).
2. Listen on TCP.
3. Start accept loop goroutine via `wg.Go`.
4. Start shutdown goroutine that closes listener on ctx cancel.
5. Return.

### Connection handling

Connections are handled by `net/http.Server`.  The handler
registered on the mux dispatches based on the request path:

1. `GET /`: respond with source table, close connection.
2. `GET /<path>`: look up mountpoint.
   - 404 if not found.
   - 401 if auth required and missing/invalid.
   - Detect NTRIP version from `Ntrip-Version` header.
   - V1: hijack connection, write `ICY 200 OK\r\n\r\n`, stream.
   - V2: write headers via `ResponseWriter`, stream with
     `ResponseController.Flush()`.

### Streaming

The streaming logic is the same for both v1 and v2 -- only the
writer differs (`net.Conn` for v1, `http.ResponseWriter` for v2).
Both paths call a common function that takes an `io.Writer` and
an optional flush function:

1. Subscribe to bcast.
2. Loop: receive packet from subscription channel.
3. Skip if not RTCM (`!pkt.HasTag(gpsreg.TagRTCM)` or
   `!pkt.ChecksumValid`).
4. If `mount.MSM7to4` and packet is MSM7: parse, convert,
   re-serialize.  Non-MSM7 packets pass through unchanged.
5. Write packet data.  Flush if v2.
6. On write error or ctx cancel: unsubscribe, return.

### MSM7→MSM4 conversion

For a mountpoint with `msm7to4 = true`:

```go
mt := rtcmbin.ExtractMsgType(pkt.Data)
if mt.IsMSM() && mt%10 == 7 {
    m7, err := rtcmbin.ParseMsg(pkt.Data)
    if err != nil { /* forward original */ }
    m4, err := rtcmbin.MSM7Convert(m7.(*rtcmbin.MSMHiRes), 4)
    if err != nil { /* forward original */ }
    data, err := rtcmbin.SerializeMsg(m4)
    if err != nil { /* forward original */ }
    // write data instead of pkt.Data
}
```

On conversion error, forward the original packet unchanged -- a
degraded MSM7 is better than no data.

## Daemon integration

In `time/app/daemon/`:

1. Add `Ntrip ntrip.Config` to the daemon `Config` struct, with
   TOML tag `toml:"ntrip"`.
2. In `run()`, after the packet bcast is created, call
   `ntrip.Start(ctx, lg, wg, cfg.Ntrip, pktBcast)` if any
   mountpoints are configured.
3. No `portLock` needed -- the caster is read-only.

## Documentation

Update `docs/man/satpulse.toml.5.md` with a new `ntrip` section
documenting the `[ntrip]`, `[[ntrip.user]]`, and
`[[ntrip.mountpoint]]` tables.

Update the JSON schema in `config-schema.json`.

## Testing

Tests use `net.Pipe` or a local TCP listener on localhost.

- Source table: connect, `GET /`, verify source table lists
  mountpoints.
- Stream: connect, `GET /RTCM`, send RTCM packets into bcast,
  verify they arrive on the connection.
- Auth: verify 401 when credentials are wrong, 200 when correct.
- MSM7→MSM4: send MSM7 packet into bcast, verify client receives
  MSM4.
- Mountpoint not found: verify 404.
- Client disconnect: verify goroutine cleans up.
- Anonymous access: no users defined, verify stream works without
  auth.

## Implementation order

1. Config types and validation.
2. Source table handler.
3. Stream handler (without MSM7→MSM4).
4. Auth.
5. MSM7→MSM4 conversion in stream handler.
6. Daemon integration and config documentation.
7. Tests throughout.
