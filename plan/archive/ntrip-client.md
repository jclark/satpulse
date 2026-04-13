# NTRIP client: NTRIPSource

Add `NTRIPSource` to `gps/app/stream` implementing the existing
`Source` interface.  This is a minimal NTRIP v1 client: it sends
a v1 request and accepts only `ICY 200 OK` as success.  v2
support can be added later as an explicit option if a caster
requires it; RTKLIB's client is v1-only and works with every
public caster in practice.

No shared NTRIP-header library.  If/when the caster
(`plan/ntrip-caster.md`) is built, any shared parsing can be
factored out then; for now the client's handshake is
self-contained.

Prerequisites (both landed, in `plan/archive/`):
`corrsink-rename.md`, `stream-backoff.md`.

## Source interface change

Change `Source.Connect` in `gps/app/stream/pull.go` from
returning `net.Conn` to returning `io.ReadCloser`:

```go
type Source interface {
    Connect(ctx context.Context) (io.ReadCloser, error)
}
```

`Pull` only calls `Read` and `Close` on the returned value, so
`io.ReadCloser` is the right abstraction.  `TCPSource` needs no
change: `net.Conn` already satisfies `io.ReadCloser`.  `Pull.Run`
and `Pull.reader` need to replace `net.Conn` parameter/var types
with `io.ReadCloser` (trivial -- all usage is `.Close()` and
passing to `scan.New` which takes `io.Reader`).

## NTRIPSource

New file `gps/app/stream/ntrip_source.go`.

```go
type NTRIPUserAgent struct {
    Version string // e.g. "1.2.3"
}

type NTRIPSource struct {
    Addr       string // "host:port"
    Mountpoint string
    Username   string
    Password   string
    UserAgent  NTRIPUserAgent
}

func (s *NTRIPSource) Connect(ctx context.Context) (io.ReadCloser, error)
```

### User-Agent

The caller populates `UserAgent.Version` from `cmd.Version()`
(the same string `--version` prints).  The header is built as
`NTRIP SatPulse/<Version>`, or just `NTRIP SatPulse` when
`Version` is empty.  NTRIP requires the User-Agent to start
with `NTRIP `.  `NTRIPUserAgent` is a struct (rather than a
bare field) so future members -- e.g. a product name override
-- can be added without changing the `NTRIPSource` surface.

### Request

Send an NTRIP v1 (HTTP/1.0) request.  No `Host` header, no
`Ntrip-Version` header -- matches what RTKLIB's client sends and
what v1 casters expect.

```
GET /<Mountpoint> HTTP/1.0\r\n
User-Agent: NTRIP SatPulse[/<Version>]\r\n
[Authorization: Basic <base64(Username:Password)>\r\n]
\r\n
```

- `Authorization` is included only when `Username != ""`.
- Base64 encoding via `encoding/base64.StdEncoding.EncodeToString`
  on the bytes of `Username + ":" + Password`.

### Response handling

v1 success is exactly the literal status line `ICY 200 OK\r\n`
followed immediately by the RTCM body.  No HTTP headers follow.
Anything else is an error.

1. Dial TCP via `net.Dialer.DialContext(ctx, "tcp", Addr)`.
2. Arrange `ctx` cancellation during the handshake: start a
   goroutine that calls `conn.Close()` on `ctx.Done()`; stop it
   once the handshake completes (success or failure).  The dial
   itself is covered by `DialContext`; this covers the following
   write/read.
3. Write the request.
4. Wrap the conn in `bufio.NewReaderSize(conn, 4096)`.  Read
   the status line with `br.ReadString('\n')`.  The 4 KB size
   bounds the maximum accepted first-line length, which is more
   than enough for any real NTRIP status or error line.
5. If `line != "ICY 200 OK\r\n"`: close the conn, return
   `fmt.Errorf("NTRIP: %s", strings.TrimSuffix(line, "\r\n"))`.
   `ReadString` may return with `err != nil` if no `\n` is seen
   before the bufio buffer fills; surface that error directly
   (conn closed).
6. On success: if `br.Buffered() == 0`, return the raw `conn`
   (no over-read).  Otherwise, `br.Peek(br.Buffered())` returns
   the leftover body bytes; return an `io.ReadCloser` that
   serves those bytes first, then the raw conn:

   ```go
   leftover, _ := br.Peek(br.Buffered())
   return struct {
       io.Reader
       io.Closer
   }{io.MultiReader(bytes.NewReader(leftover), conn), conn}, nil
   ```

   The body is read directly from `conn` after the leftover is
   drained -- the bufio reader is not used for body reads.

No concurrency in `Connect` beyond the ctx-cancel goroutine --
`Connect` is called synchronously by Pull's reader goroutine,
one call at a time.

## Testing

New file `gps/app/stream/ntrip_source_test.go`.  Use a local TCP
listener on localhost to script responses and verify the client.
Similar in spirit to the `pipeSource` harness in `pull_test.go`
but with a real listener so `NTRIPSource` can dial it.

Cases:

- **v1 handshake**: listener sends `"ICY 200 OK\r\n<RTCM bytes>"`;
  verify `Connect` succeeds and the returned reader yields the
  RTCM bytes correctly (covers both the no-over-read and
  with-over-read paths by varying how the listener writes).
- **Request headers**: listener captures the request bytes;
  verify `GET`, HTTP/1.0, `User-Agent`, and `Authorization`
  (when creds set) are present with expected values.
- **User-Agent formatting**: `Version == ""` -> `"NTRIP SatPulse"`;
  `Version == "1.2.3"` -> `"NTRIP SatPulse/1.2.3"`.
- **Auth header**: `Username="user"`, `Password="pw"` -> base64
  of `"user:pw"`.  No `Authorization` header when `Username`
  empty.
- **Error response**: listener sends
  `"ERROR - Bad Password\r\n"`; verify `Connect` returns an
  error whose message contains `"ERROR - Bad Password"`.
- **HTTP error response**: listener sends
  `"HTTP/1.1 401 Unauthorized\r\n..."`; verify `Connect` returns
  an error whose message contains the status line.
- **ctx cancellation mid-handshake**: cancel ctx after dial but
  before listener responds; verify `Connect` returns promptly
  with an error.
- **Connection refused** (no listener on the port): verify
  `Connect` returns the dial error.
