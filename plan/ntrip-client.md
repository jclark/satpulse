# NTRIP client: NTRIPSource and ntriphdr library

Add `NTRIPSource` to `gps/app/stream` implementing the existing
`Source` interface.  Add `gps/lib/ntriphdr` for shared NTRIP
header parsing used by both `app/stream` and `app/ntrip` (caster).

Prerequisite: `plan/corrsink-rename.md` (rename corrsink to stream)
and `plan/stream-backoff.md` (adaptive backoff).

## gps/lib/ntriphdr

Pure library package, no goroutines or logging.

- Detect NTRIP version from response status line: distinguish
  `ICY 200 OK` from `HTTP/1.1 200 OK`.
- Detect NTRIP version from request headers: check for
  `Ntrip-Version: Ntrip/2.0`.
- Format and parse `SOURCE <password> /<mountpoint>` request
  lines (used by Push later, accepted by the caster).
- Parse source table entries (STR, CAS, NET lines).
- Constants: standard header names, version strings.

## NTRIPSource

New file `ntrip_source.go` in `gps/app/stream`.  Implements
`Source` so it plugs directly into `Pull.Run` with no changes
to the Pull pipeline.

```go
type NTRIPSource struct {
    Addr       string // "host:port"
    Mountpoint string
    Username   string
    Password   string
}

func (s *NTRIPSource) Connect(ctx context.Context) (net.Conn, error)
```

`Connect` does:

1. Dial TCP to Addr using `net.Dialer.DialContext`.
2. Send `GET /<Mountpoint> HTTP/1.1\r\n` with headers:
   `Host`, `Ntrip-Version: Ntrip/2.0`, `User-Agent`,
   and `Authorization: Basic ...` (if credentials set).
3. Read the response status line using `ntriphdr`.
4. If `ICY 200 OK` or `HTTP/1.1 200 OK`: return the net.Conn.
   The caller (Pull's reader) reads raw RTCM bytes from it.
5. On error status: close connection, return error.

No concurrency -- `Connect` is called synchronously by Pull's
reader goroutine, one call at a time.

## Testing

- `ntriphdr`: unit tests for parsing status lines, version
  detection, SOURCE line formatting, source table parsing.
- `NTRIPSource`: test using a local TCP listener that speaks
  enough NTRIP to verify the handshake.  Test both v1 (ICY)
  and v2 (HTTP/1.1) responses.  Test auth failure (401).
  Test bad mountpoint (404).
