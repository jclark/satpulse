# Ntrip NMEA send support: client GGA upload (#325)

## Motivation

A position-dependent caster synthesizes corrections for the client's own
location, so the client must upload its position as an NMEA GGA sentence before
the caster will stream. In the Ntrip spec (v1 sec 5.5.3, v2 sec 2.1.3), when a
mountpoint's source-table `<nmea>` field is `1`, the caster needs
at least one GGA to prepare data and start sending. u-blox PointPerfect's Ntrip
service is position-dependent, so `stream.pull.ntrip` needs an NMEA send mode that uploads a current
GGA.

Scope notes:

- This plan covers only the production `stream.pull.ntrip.nmeaSend` work.
- It depends on the NMEA decode layer (#330, typed GGA parsing/serialisation in
  `nmeamsg`) and the GGA synthesis and selected-GGA core recorded in
  [nmea-gga.md](nmea-gga.md). This plan owns the NMEA-send-specific wiring of
  that selector into the daemon and stream pull path.
- It does not depend on [nmea-gga-tcp.md](../nmea-gga-tcp.md); enabling NMEA
  send must not require any proxy service or `proxy.tcp synth` option.
- Our own caster (`gps/app/ntrip`) is a physical base: field 12 `<nmea>` is
  hard-coded `"0"` in `strrec.go`. Nothing here changes that.
- The spec requires only one GGA to start, but testing showed many
  position-dependent casters stop streaming unless they keep receiving GGA, so
  in practice we re-upload periodically (see `nmeaSendInterval`). Sending more
  than one GGA is always spec-allowed.
- The completed `satpulsetool ntrip --gga` diagnostic proved that u-blox
  PointPerfect accepts our Ntrip v1 request flow.

## Config

```toml
[stream.pull.ntrip]
address = "caster.example.com:2101"
mountpoint = "GGA"
username = "user"
password = "pass"
nmeaSend = true
nmeaSendInterval = 5
```

`nmeaSend = true` is meaningful only for ntrip pull.

`nmeaSendInterval` is the GGA upload interval in seconds (`*float64` so an
unset key is distinguishable from `0`). Many position-dependent casters drop
the stream unless they keep receiving GGA, so the default behaviour is periodic
re-upload, not one-shot. An unset key defaults to 5 seconds; a configured `0`
means upload once per connection. The value has effect only on the `nmeaSend`
path, but it is validated whenever the key is present, regardless of `nmeaSend`:
a non-finite, negative, sub-1-second (but non-zero), or above-maximum value is a
config error. Validating eagerly catches a typo'd interval even when the key is
left in a config with `nmeaSend` off.

## Implementation

1. `Source.Connect` returns a stream package interface instead of
   `io.ReadCloser`:

   ```go
   type ReadWriteDeadlineCloser interface {
    io.Reader
    io.Writer
    io.Closer
    SetWriteDeadline(time.Time) error
   }
   ```

   This gives `ggaSender` the writer half after the Ntrip handshake and a way to
   bound GGA writes without setting read deadlines on the correction stream.
   `TCPSource` can return the `net.Conn` directly. `NtripSource` can also return
   the `net.Conn` directly when the handshake reader has no buffered body bytes.
   When `bufio.Reader` has already buffered correction-stream bytes, `NtripSource`
   returns a wrapper whose `Read` drains
   `io.MultiReader(bytes.NewReader(leftover), conn)`, while `Write`, `Close`, and
   `SetWriteDeadline` forward to the underlying `conn`.

2. `stream.NtripConfig` grows an `NMEASend bool` field with TOML tag `nmeaSend`.
   `PullConfig.NewPull` is only a TOML convenience wrapper: it builds the `Source`,
   resolves `nmeaSendInterval`, and calls `stream.NewPull`. Dynamic callers such
   as the desktop GUI can build their own `Source` and call `stream.NewPull`
   directly.

3. `time/app/daemon` is the NMEA send integration point for the selected-GGA core.
   After building `stream.Pull` and before creating `gpsevent.Dispatcher`, the
   daemon checks whether the configured pull uses NMEA send. If so, it creates
   the stage 3 `stream.GGASelector`, keeps `selector.Packets()` for `Pull.Run`,
   and passes the selector into `gpsevent.NewDispatcher` as an optional
   receiver-side `GGASelector` interface with `Packet` and `GGASentence`, not the
   concrete stream type.

4. When that optional sink is present, `gpsevent.Dispatcher` owns a
   `nmeasyn.Synth` whose sink is the same interface. The dispatcher fans out the
   same decoded `gpsprot` message callbacks it already receives to the
   synthesizer, in the same order, so `nmeasyn` sees the complete
   `TimeMsg`/position/`NavEpochMsg` epoch sequence. `Dispatcher.handlePacket`
   feeds each successfully processed receiver packet to `sink.Packet`; the
   selector filters that stream to checksum-valid approved NMEA GGA packets. That
   keeps receiver GGA ahead of synthesized GGA for the same UTC without making
   `gpsevent` depend on the selector implementation.

5. `Pull.Run` takes an optional selected-GGA receive channel. This is the receive
   side of the nonblocking capacity-1 latest-value channel owned by the selector.
   A nil channel disables NMEA send for callers that do not need GGA upload.

6. In NMEA send mode, pull owns a `ggaSender` goroutine. It consumes the selected-GGA
   feed built by `time/app/daemon` from receiver NMEA plus synthesized fill-ins.
   Receiver GGA wins for a UTC whenever present.

7. `ggaSender` owns all mutable state: `currentWriter` plus the latest suitable
   GGA wire bytes and a one-shot readiness signal. `NewGGASender` takes the
   resolved interval; when it is > 0 the sender runs a `time.Ticker`, otherwise
   the tick channel is nil and never fires. It selects on:
   - selected-GGA feed: parse the GGA to confirm it is usable (quality > 0, lat
     and lon set), hold the original wire bytes as latest, and signal readiness
     the first time it has a usable packet. It does not write on this case;
     receiving a new GGA only updates the held latest;
   - the ticker: if connected, write the held latest GGA bytes with a deadline;
   - `connCh`: adopt the new writer after each successful dial and immediately
     force-send the held latest GGA;
   - `ctx.Done()`: exit.

   The ticker is the only periodic driver and `connCh` the only immediate
   driver, so a stationary client still gets the periodic keepalive every
   `nmeaSendInterval` seconds. When the receiver loses its fix the sender keeps
   re-sending the last usable GGA, which keeps the stream alive.

8. The existing `reader()` reconnect loop remains the only code that dials. On
   startup in NMEA send mode it waits for `ggaSender`'s readiness signal, rather than
   consuming the selected-GGA feed itself. On each successful connect, it sends
   the new writer to `ggaSender` over `connCh`. Reconnects therefore resend the
   latest GGA even for a stationary client.

9. In NMEA send mode, `reader()` does not dial until there is a selected GGA with
   quality > 0, signalled by `ggaSender`. Later reconnects use the held latest
   GGA immediately.

10. Periodic re-upload supersedes the earlier significant-movement gate. Repeat
   sends come only from the `nmeaSendInterval` ticker (plus the per-connection
   force-send), so there is no distance computation: a stationary client is
   re-sent on every tick, and `nmeaSendInterval = 0` means once per connection
   even if the client moves.

11. After `ggaSender` exists, switch `satpulsetool ntrip` onto the same
   post-handshake GGA sender used by NMEA send. The command builds one literal
   validated GGA from `--nmea-send-pos` and feeds it through a one-shot
   selected-GGA channel instead of using a separate static `NtripSource.GGA`
   mechanism. The shared sender's quality > 0 gate means a quality-0 GGA no
   longer starts a connection. A `--nmea-send-interval secs` flag sets the
   sender interval (default 5 to match the daemon, `0` uploads once on connect)
   so the diagnostic exercises the same periodic flow real casters need. This
   leaves only one code path that writes GGA to an Ntrip caster.

Behavioral note to document: in NMEA send mode pull does not connect until a valid
position fix exists, so corrections are gated on the receiver first achieving a
standalone fix. Log that state when NMEA send pull starts waiting, e.g. "NMEA send pull
waiting for first position fix before connecting", so an operator can diagnose a
silent correction stream when the receiver never gets its first fix.

## Tests

- Config parsing/validation for `nmeaSend = true`, `nmeaSendInterval` parsing,
  the unset-defaults-to-5s and explicit-0 resolution, and rejection of a
  negative/non-finite interval.
- NMEA send pull waits for a valid selected GGA before connecting.
- A blocked GGA sender does not backpressure dispatcher processing; selected GGA
  delivery drops stale pending GGA under pressure.
- The reader waits on the `ggaSender` readiness signal and never consumes the
  selected-GGA feed directly.
- Ntrip connect with buffered body bytes preserves those reads while forwarding
  GGA writes and write deadlines to the underlying connection.
- Force-send selected GGA on connect and reconnect.
- Receiver GGA is sent verbatim when present.
- Synthesized GGA can be used when the receiver does not emit GGA.
- Periodic re-upload: a positive interval re-sends the held GGA even for a
  stationary client; `nmeaSendInterval = 0` uploads once per connection and a
  later move does not trigger another send.
- Quality 0 GGA does not start NMEA send connection.
- `satpulsetool ntrip` sends via the same post-handshake GGA sender as NMEA
  send, with a one-shot literal-GGA input, and `--nmea-send-interval` sets the
  re-send period.
- `satpulsetool ntrip` with a quality-0 literal does not start the connection
  after it moves to the shared sender.
- Write-deadline and write-error handling drops the writer and lets the reader
  reconnect path recover.
- The `smoketest/scenarios/stream/nmea-send` scenario uses a fake correction
  source that waits for GGA before streaming corrections, and (with a small
  `nmeaSendInterval`) asserts the daemon re-sends GGA periodically, not just
  once on connect.

## Open decisions

- None outstanding. A 1-second minimum `nmeaSendInterval` is enforced (a
  configured `0` is still allowed and means once per connection). The effective
  re-send rate is in any case bounded by the nav-epoch interval that drives the
  selected-GGA feed, so a sub-second value could not make the held GGA churn
  faster than the receiver produces fixes; the minimum simply rejects a
  nonsensical config up front rather than silently clamping it.
