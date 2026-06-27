# Ntrip VRS support: client GGA upload (#325)

## Motivation

A Virtual Reference Station (VRS) caster synthesizes corrections for the
client's own location, so the client must upload its position as an NMEA GGA
sentence before the caster will stream. In the Ntrip spec (v1 sec 5.5.3, v2 sec
2.1.3), when a mountpoint's source-table `<nmea>` field is `1`, the caster needs
at least one GGA to prepare data and start sending. u-blox PointPerfect's Ntrip
service is a VRS, so `stream.pull.ntrip` needs a VRS mode that uploads a current
GGA.

Scope notes:

- This plan covers only the production `stream.pull.ntrip.vrs` work.
- It depends on the NMEA decode layer (#330, typed GGA parsing/serialisation in
  `nmeamsg`), stage 2 of `plan/nmea-gga.md` (GGA synthesis from `gpsprot`
  messages), and stage 3 of `plan/nmea-gga.md` (the selected-GGA feed).
- It does not depend on stage 4 of `plan/nmea-gga.md`; enabling VRS must not
  require any proxy service or `proxy.tcp synth` option.
- Our own caster (`gps/app/ntrip`) is a physical base, never a VRS: field 12
  `<nmea>` is hard-coded `"0"` in `strrec.go`. Nothing here changes that.
- The spec requires only one GGA to start; sending more is allowed for a moving
  client. There is no periodic-GGA keepalive requirement for the HTTP/TCP
  streaming flow we use.
- The completed `satpulsetool ntrip --gga` diagnostic proved that u-blox
  PointPerfect accepts our Ntrip v1 request flow.

## Config

```toml
[stream.pull.ntrip]
address = "caster.example.com:2101"
mountpoint = "VRS"
username = "user"
password = "pass"
vrs = true
```

`vrs = true` is meaningful only for ntrip pull.

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

2. `stream.NtripConfig` grows a `VRS bool` field with TOML tag `vrs`.
   `PullConfig.Prepare` records whether the prepared pull is VRS mode, but does
   not build any GGA selector itself.

3. `PullSetup` grows an optional selected-GGA input, set by `time/app/daemon`
   after the daemon creates the stage 3 selector from `plan/nmea-gga.md`. This
   is the receive side of the nonblocking capacity-1 latest-value channel owned
   by the selector. `PullSetup.Run` passes that receive-only channel into
   `Pull.Run`.

4. In VRS mode, pull owns a `ggaSender` goroutine. It consumes the selected-GGA
   feed built by `time/app/daemon` from receiver NMEA plus synthesized fill-ins.
   Receiver GGA wins for a UTC whenever present.

5. `ggaSender` owns all mutable state: `(currentWriter, lastSentLatLon)` plus
   the latest suitable GGA packet and a one-shot readiness signal. It selects on:
   - selected-GGA feed: parse enough GGA fields to get quality and lat/lon, hold
     the original wire packet as latest, signal readiness the first time it has a
     quality > 0 packet, and if connected and moved enough, write those bytes
     with a deadline;
   - `connCh`: adopt the new writer after each successful dial and immediately
     force-send the held latest GGA;
   - `ctx.Done()`: exit.

6. The existing `reader()` reconnect loop remains the only code that dials. On
   startup in VRS mode it waits for `ggaSender`'s readiness signal, rather than
   consuming the selected-GGA feed itself. On each successful connect, it sends
   the new writer to `ggaSender` over `connCh`. Reconnects therefore resend the
   latest GGA even for a stationary client.

7. In VRS mode, `reader()` does not dial until there is a selected GGA with
   quality > 0, signalled by `ggaSender`. Later reconnects use the held latest
   GGA immediately.

8. Significant-change gate: use a 2D flat-earth/equirectangular distance from
   parsed GGA lat/lon. Ignore height. A tens-of-metres threshold is enough.

9. After `ggaSender` exists, switch `satpulsetool ntrip --gga` onto the same
   post-handshake GGA sender used by VRS. The command still accepts one literal
   validated GGA and sends it once, but it feeds that packet through a one-shot
   selected-GGA channel instead of using a separate static `NtripSource.GGA`
   mechanism. The shared sender's quality > 0 gate means a pasted quality-0 GGA
   no longer starts a connection; the diagnostic becomes "upload a usable GGA
   once" rather than "write any syntactically valid GGA once". This leaves only
   one code path that writes GGA to an Ntrip caster.

Behavioral note to document: in VRS mode pull does not connect until a valid
position fix exists, so corrections are gated on the receiver first achieving a
standalone fix. Log that state when VRS pull starts waiting, e.g. "VRS pull
waiting for first position fix before connecting", so an operator can diagnose a
silent correction stream when the receiver never gets its first fix.

## Tests

- Config parsing/validation for `vrs = true`.
- VRS pull waits for a valid selected GGA before connecting.
- A blocked GGA sender does not backpressure dispatcher processing; selected GGA
  delivery drops stale pending GGA under pressure.
- The reader waits on the `ggaSender` readiness signal and never consumes the
  selected-GGA feed directly.
- Ntrip connect with buffered body bytes preserves those reads while forwarding
  GGA writes and write deadlines to the underlying connection.
- Force-send selected GGA on connect and reconnect.
- Receiver GGA is sent verbatim when present.
- Synthesized GGA can be used when the receiver does not emit GGA.
- Significant-move gate sends on movement and stays quiet when stationary.
- Quality 0 GGA does not start VRS connection.
- `satpulsetool ntrip --gga` sends via the same post-handshake GGA sender as
  VRS, with a one-shot literal-GGA input.
- `satpulsetool ntrip --gga` with a quality-0 literal does not start the
  connection after it moves to the shared sender.
- Write-deadline and write-error handling drops the writer and lets the reader
  reconnect path recover.
- Add a `smoketest/scenarios/stream/vrs` scenario (`.py`, `.toml.in`,
  `SCENARIOS` entry, and README list entry) using a fake VRS caster that waits
  for GGA before streaming corrections.

## Open decisions

- Significant-move threshold value, and whether it is configurable.
