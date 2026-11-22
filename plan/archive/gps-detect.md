# Improved GPS detection

## Current implementation

In the current code, "detection" means only listening for packets. Detection and probing are separate sequential phases: detection listens for packets first, and only if detection succeeds and probing is needed does a separate probe phase follow.

### satpulsed

**When `config = false` (default):**
- Detection runs (listens for valid GPS messages)
- No probe packets are sent
- Detection failure logs a warning but daemon continues
- Receiver vendor/model remains unknown

**When `config = true` with PHC clock:**
- Detection runs
- If detection fails: daemon exits with error
- If detection succeeds: probing and configuration proceed
- Probe timeout is non-fatal (logs INFO, continues without configuring)

**When `config = true` without PHC clock:**
- Detection runs
- If detection fails: logs warning, daemon continues
- If detection succeeds: probing and configuration proceed

### satpulsetool gps

**Default behavior (no flags):** runs `--show-receiver` automatically.

**`--show-receiver`:** Detection, then probe, then print receiver info. Detection failure is fatal. If detection succeeds but probe times out, exits successfully without printing receiver info.

**Configuration flags (`--pps`, `--save`, etc.):** Detection, then probe, then configure. Detection failure is fatal. Probe timeout is fatal.

**`--socket`:** Skips detection when probing/configuring (daemon already validated the connection). Has no effect in passive capture mode.

**`--force-probe`:** Attempts probing even when detection fails.

**`--capture` with `--packet-log` only:** Passive mode - just captures packets, no detection or probing.

### Detection

Detection listens for a suitable packet (any packet with a valid checksum from a non-native-only protocol, i.e. not RTCM).

Two fixed timers start when detection begins:
- 2s timer
- 15s timer

Detection exits when any of these is true:
- A suitable packet is received (success)
- The 15s timer fires (give up)
- The 2s timer fires AND no framing errors have occurred (give up)

On failure, the error message describes what was observed: framing errors ("wrong speed?"), no output at all, corrupt messages ("multiple processes reading?"), only native-only protocols, or unparseable output.

### Probing

Probing runs as a separate phase after detection. It sends a single probe packet for every registered config protocol simultaneously, then waits up to 1.5s for any protocol to report `ProbeOK()`. The first protocol to respond wins.

After probing (whether it succeeded or timed out), ongoing errors are checked: the error counters are snapshotted before probing starts, and any new framing errors or corrupt messages that occurred during probing are fatal.

## Problems

### Code is complex and poorly documented

The current code has been developed incrementally, with features like `ForceProbe` flags and the `NoOp()` check bolted on as needs arose. The detection and probing logic is spread across `Configure()`, `detect()`, and `probe()` with complex control flow. The interaction between these is hard to follow.

### No probing if detection fails

This is issue #36. Because detection and probing are sequential, detection failure means probe packets are never sent. Silent receivers (those that produce no output until configured) cannot be identified or configured. The only workaround is `--force-probe` in satpulsetool.

### Only one probe is sent

The single probe with a 1.5s timeout is sometimes unreliable. We should retry.

### config=false with PHC should be fatal

When `config = false`, there is a PHC clock, and detection fails, the daemon logs a warning and continues. This is wrong: time synchronization cannot work without GPS output, so this should be fatal. (Without a PHC clock, continuing is correct because a TCP proxy may configure the receiver.)

### ErrNoProbeResponse when no config protocols registered

When no config protocols are registered but `NoOp()` is false, the current code still enters the probing path. `probe()` returns `nil, nil` (no protocols to probe), and `Configure()` returns `ErrNoProbeResponse`. This makes no sense: you cannot fail to get a probe response when you never probed.

### Timeout values not well thought through

For example, the 15s timeout when framing errors occur is overly generous.

## New design

Rather than patching the problems individually, this is a rethink of the whole detection and probing approach, aiming for a coherent, well-documented design that addresses all of the above problems together.

### Terminology

- **Detection** is the overall process of determining what GPS receiver we are talking to. It always involves listening for packets. It may also involve probing.
- **Listening** means reading packets from the GPS and classifying them.
- **Probing** means sending probe packets and waiting for a response that identifies the receiver.
- A **suitable packet** is a packet from a non-native-only protocol (i.e. not RTCM). This confirms the receiver supports a protocol that can provide time information, making the connection potentially usable for time synchronization.
- A **valid packet** is any packet with a correct checksum, including RTCM.

### Two modes

Detection runs in one of two modes:

- **Listening only** (probing disabled): wait for a suitable packet, then return.
- **Listening + probing** (probing enabled): listen for packets, send probes, exit when probe succeeds or times out.

Probing is enabled in the same situations as now: when there is configuration to apply (or `--show-receiver` forces it) and config protocols are registered.

### Deadline and framing errors

Framing errors may indicate a wrong baud rate, but practical experience with many GPS receivers and Linux systems shows there is often a period of a few seconds at the beginning when we get framing errors even at the correct baud rate. So we need to allow time for this to settle before giving up.

Both modes use a deadline timer. It starts at 2s. Each time a framing error occurs, the deadline is extended to 2s from now, but never beyond 10s from detection start. In listening mode, the deadline is the "give up" timer. In probing mode, the deadline triggers the first probe if nothing else does first; once the first probe is sent, the deadline is disabled and probe timing takes over.

### Silence detection

Some receivers produce no output in their factory default state (notably Unicore UM980). Receivers that do produce output do so at least once per second. A silence timer detects the "nobody home" situation so we can probe early rather than waiting for the full deadline. The silence timer starts at 1s when detection begins. Any input (data or framing errors) cancels it permanently: once we know something is arriving on the serial link, the silence question is answered.

### Probing strategy

The first probe is sent on the earliest of:
- 1s of silence (no input received at all)
- a valid packet is received
- the deadline fires

If no probe response in 1.5s, send a second probe. Never send more than two probes. Give up 3s after the second probe.

### Exit conditions

**Listening only:**
- Suitable packet received: success.
- Deadline fires: give up.

**Probing:**
- ProbeOK: success.
- Probe timed out (3s after second probe): give up probing.

### Results

**Listening only:**
- Suitable packet seen: success.
- No suitable packet seen: return ErrNotDetected.

**Probing:**
- Probe succeeded: success.
- Probe failed, suitable packets seen: return ErrNoProbeResponse. The caller decides whether this is fatal (satpulsetool with configuration requested) or non-fatal (satpulsed, or satpulsetool with `--show-receiver` only).
- Probe failed, no suitable packets seen: return ErrNotDetected.

The ErrNotDetected message describes what was observed: framing errors ("wrong speed?"), no output at all, corrupt messages ("multiple processes reading?"), or only native-only protocols.

### Ongoing error tracking

When the first valid packet is received, we snapshot the error counters. Any framing errors or corrupt messages after that point are "ongoing errors" and are logged as a warning. They do not change the detection result.

### Socket case

When connected via socket, the daemon has already validated the connection. Probe immediately, skip the "when to send first probe" logic, and skip ongoing error checking and detection validation.

## Interface changes

These changes are to files outside gpscfg.go.

### Simplify ForceProbe

In the current code, `ForceProbe` is a bitmask type with two constants:
- `ForceProbeWhenNoOutput` - set by `--force-probe` flag, allows probing even when detection fails
- `ForceProbeWhenNoConfig` - set automatically for `--show-receiver`, forces probing even when there is no configuration to apply

With the new design, probing happens during detection, so `ForceProbeWhenNoOutput` is no longer needed. This leaves only `ForceProbeWhenNoConfig`, so `ForceProbe` is simplified from a bitmask to a simple `bool` in `ConfigOptions`.

The `--force-probe` flag is removed from satpulsetool gps and its man page.

### Rename Detected to Socket

`ConfigOptions.Detected` is renamed to `ConfigOptions.Socket`. The old name was confusing because "detected" in the new design refers to the detection process, but this flag actually means "connected via socket, skip serial detection".

## Implementation

This section describes the implementation in gpscfg.go. The core of the change is a new `detect()` function that replaces the old `detect()`, `probe()`, and the detection/probing logic in `Configure()`.

### Configure()

`Configure()` calls `detect()` to handle both listening and probing in a single step. Native message handlers are installed before detection when probing is enabled, so that probe responses are routed correctly.

```go
func Configure(ctx context.Context, ...) (*Result, error) {
    mh := msgHandler{}
    mh.init(lg, packetProcs, configProts, packetCh)

    noop := target.NoOp()
    probeEnabled := !noop && len(mh.configProts) > 0
    socket := target.Opts.Socket

    var mnmh *gpsprot.MultiNativeMsgHandler
    if probeEnabled {
        var restore func()
        mnmh, restore = mh.installNativeMsgHandlers()
        defer restore()
    }

    configProt, err := mh.detect(ctx, port, probeEnabled, socket)
    if err != nil {
        return nil, err
    }
    if !probeEnabled {
        return mh.noProbeResult(), nil
    }
    if configProt == nil {
        return mh.noProbeResult(), ErrNoProbeResponse
    }

    mnmh.Reset(&mh, configProt)
    // ... configure using configProt ...
}
```

### Constants

```go
const (
    listenTimeout         = 2 * time.Second         // initial deadline (both modes)
    listenMaxTimeout      = 10 * time.Second         // absolute cap for deadline extensions
    silentWaitTimeout     = 1 * time.Second            // silence before first probe
    probeRetryDelay       = 1500 * time.Millisecond  // wait after first probe before retry
    probeResponseTimeout  = 3 * time.Second           // wait after second probe before giving up
)
```

### Structs

```go
// detector holds shared state for both detection modes.
type detector struct {
    mh          *msgHandler
    deadline    <-chan time.Time
    maxDeadline time.Time

    validPacketReceived bool
    badStart            badCount
    totalMsgCount       int
}

// listeningDetector detects by listening for a suitable packet.
type listeningDetector struct {
    detector
}

// probingDetector detects by sending probe packets and listening for responses.
type probingDetector struct {
    detector
    port         gpsio.OutPort
    silenceTimer <-chan time.Time
    probeTimer   <-chan time.Time
    nProbesSent  int
}
```

### detect()

The top-level detection function. Creates the appropriate detector based on mode, runs it, then evaluates the result. Returns `(ConfigProtocol, nil)` on success, or `(nil, error)` on failure.

```go
func (mh *msgHandler) detect(ctx context.Context, port gpsio.OutPort,
        probeEnabled bool, socket bool) (gpsprot.ConfigProtocol, error) {
    if socket && !probeEnabled {
        return nil, nil
    }
    base := detector{
        mh:          mh,
        deadline:    time.After(listenTimeout),
        maxDeadline: time.Now().Add(listenMaxTimeout),
    }
    var configProt gpsprot.ConfigProtocol
    var err error
    if probeEnabled {
        pd := &probingDetector{
            detector:     base,
            port:         port,
            silenceTimer: time.After(silentWaitTimeout),
        }
        if socket {
            if err := pd.maybeSendProbe(0); err != nil {
                return nil, err
            }
            pd.validPacketReceived = true
            pd.silenceTimer = nil
            pd.deadline = nil
        }
        configProt, err = pd.run(ctx)
    } else {
        ld := &listeningDetector{detector: base}
        err = ld.run(ctx)
    }
    if err != nil {
        return nil, err
    }
    if socket {
        return configProt, nil
    }
    // Check ongoing errors (warn, not fatal)
    if base.validPacketReceived {
        badNew := mh.bad.Sub(base.badStart)
        if badNew.framingErrs > 0 {
            mh.lg.Warn("ongoing framing errors reading GPS output (hardware problems?)")
        }
        if badNew.corruptMsgs > 0 {
            mh.lg.Warn("ongoing corrupted GPS output (multiple processes reading from serial port?)")
        }
    }
    if configProt != nil {
        return configProt, nil
    }
    // No suitable packets: ErrNotDetected with descriptive message
    if mh.suitableMessageCount() == 0 {
        // ... generate msg based on mh.bad and mh.nativeOnlyTags() ...
        return nil, fmt.Errorf("%w: %s", ErrNotDetected, msg)
    }
    // Suitable packets seen but probe failed:
    // return nil configProt; Configure() returns ErrNoProbeResponse.
    return nil, nil
}
```

### isFramingError

```go
func isFramingError(err error) bool {
    se, ok := err.(SerialError)
    return ok && se.FramingErrs() > 0
}
```

### detector.processPacket

Shared by both modes. Processes the packet, tracks valid packet receipt, and extends the deadline on framing errors.

```go
func (d *detector) processPacket(packet scan.Packet) {
    mh := d.mh
    countBefore := d.totalMsgCount
    mh.packet(packet)
    d.totalMsgCount = mh.totalMsgCount()
    if d.totalMsgCount > countBefore && !d.validPacketReceived {
        d.validPacketReceived = true
        d.badStart = mh.bad
    }
    if isFramingError(packet.ReadError) && d.deadline != nil {
        now := time.Now()
        extended := now.Add(listenTimeout)
        if extended.Before(d.maxDeadline) {
            d.deadline = time.After(listenTimeout)
        } else if now.Before(d.maxDeadline) {
            d.deadline = time.After(d.maxDeadline.Sub(now))
        }
    }
}
```

### probingDetector.processPacket

Extends the base to cancel the silence timer on input and trigger the first probe when a valid packet arrives.

```go
func (d *probingDetector) processPacket(packet scan.Packet) error {
    d.detector.processPacket(packet)
    if len(packet.Data) > 0 || isFramingError(packet.ReadError) {
        d.silenceTimer = nil
    }
    if d.validPacketReceived {
        if err := d.maybeSendProbe(0); err != nil {
            return err
        }
    }
    return nil
}
```

### listeningDetector.run

Listens for packets until a suitable one arrives or the deadline fires.

```go
func (d *listeningDetector) run(ctx context.Context) error {
    mh := d.mh
    for {
        select {
        case <-ctx.Done():
            return ctx.Err()
        case packet, ok := <-mh.packetCh:
            if !ok {
                return mh.packetChClosed(ctx)
            }
            d.processPacket(packet)
            if mh.suitableMessageCount() > 0 {
                return nil
            }
        case <-d.deadline:
            d.deadline = nil
            return nil
        }
    }
}
```

### probingDetector.run

Listens for packets while managing probe sending and timeouts.

```go
func (d *probingDetector) run(ctx context.Context) (gpsprot.ConfigProtocol, error) {
    mh := d.mh
    for {
        select {
        case <-ctx.Done():
            return nil, ctx.Err()
        case packet, ok := <-mh.packetCh:
            if !ok {
                return nil, mh.packetChClosed(ctx)
            }
            if err := d.processPacket(packet); err != nil {
                return nil, err
            }
            if prot := mh.probeSucceeded(); prot != nil {
                return prot, nil
            }
        case <-d.silenceTimer:
            d.silenceTimer = nil
            if err := d.maybeSendProbe(0); err != nil {
                return nil, err
            }
        case <-d.deadline:
            d.deadline = nil
            if err := d.maybeSendProbe(0); err != nil {
                return nil, err
            }
        case <-d.probeTimer:
            d.probeTimer = nil
            if d.nProbesSent >= 2 {
                return nil, nil // probe timed out
            }
            if err := d.maybeSendProbe(d.nProbesSent); err != nil {
                return nil, err
            }
        }
    }
}
```

### maybeSendProbe

Sends probe packets for all config protocols if `nProbesSent` equals `probeIndex`. Increments `nProbesSent`, nils `deadline` (probe timing takes over), and sets `probeTimer` to `probeRetryDelay` after the first probe or `probeResponseTimeout` after the second.

## Testing

Unit tests for detection use `testing/synctest` (Go 1.25) for deterministic time control. All tests are in `gpscfg_test.go` in package `gpscfg` (same-package tests for access to unexported types).

### Approach

Each test runs inside `synctest.Test`, which creates an isolated "bubble" with a fake clock. Within the bubble, `time.After` and `time.Now` use the fake clock, and channels created inside the bubble are "bubbled" -- blocking on them counts as durably blocked for synctest purposes.

The core pattern:

1. Start `detect()` in a goroutine inside the bubble. It enters the select loop, blocking on `packetCh` and timer channels.
2. `synctest.Wait()` returns when detect is durably blocked (waiting in select).
3. Send a packet to `packetCh` or advance time with `time.Sleep(d)` to fire a timer.
4. `synctest.Wait()` again to let detect process the event and re-block.
5. Inspect side effects (probe writes, return values, errors).

### Fake types

**`fakeOutPort`** implements `gpsio.OutPort`. Only `Write` does anything: it records all writes for assertion. `Buffered` returns 0, `TransmitTime` returns 0.

**`fakeConfigProtocol`** implements `gpsprot.ConfigProtocol`. `ProbePacket()` returns a fixed byte slice. `ProbeOK()` returns a controllable boolean. `NativeMsg` is a no-op. `Configure` panics (never called in detection tests).

**`fakeSerialError`** implements `SerialError`. Returns a configurable `FramingErrs()` count.

### Helpers

**`makeNMEAPacket`** constructs a valid `scan.Packet` with a GPZDA sentence and correct NMEA checksum, using `nmea.PacketFormat` and `nmeamsg.Checksum`. This counts as a suitable packet (NMEA is not native-only).

**`makeFramingErrorPacket`** constructs a `scan.Packet` with a `fakeSerialError` and some invalid data.

**`setupMsgHandler`** creates a `msgHandler` with real packet processors from `gpsreg.CreatePacketProcessors(nil)` and a buffered `packetCh`. Takes an optional slice of config protocols.

**`runDetect`** starts `detect()` in a goroutine, returning results through a channel.

### Test 1: listening-only success

Goal: a suitable packet causes immediate success in listening mode.

1. `setupMsgHandler(nil)`, `probeEnabled=false`.
2. Start detect, `synctest.Wait()`.
3. Send a valid NMEA packet.
4. `synctest.Wait()`, receive result.
5. Assert `err == nil`, `configProt == nil`.

### Test 2: listening-only timeout with framing errors

Goal: framing errors extend the deadline up to 10s, then detect gives up.

1. `setupMsgHandler(nil)`, `probeEnabled=false`.
2. Start detect, `synctest.Wait()`.
3. In a loop: `time.Sleep(1500ms)`, send framing error packet, `synctest.Wait()`. Repeat until past 10s.
4. After the max deadline expires, receive result.
5. Assert `err` wraps `ErrNotDetected` with "framing" in the message.

### Test 3: probing triggered by silence

Goal: 1s of silence triggers the first probe.

1. Setup with a `fakeConfigProtocol`, `probeEnabled=true`, call `installNativeMsgHandlers`.
2. Start detect with `fakeOutPort`, `synctest.Wait()`.
3. `time.Sleep(1s)`, `synctest.Wait()` -- the silence timer fires, probe is sent.
4. Assert `fakeOutPort` has exactly 1 write matching the probe packet.
5. Close `packetCh` to let detect finish.

### Test 4: probing triggered by valid packet

Goal: a valid packet triggers an immediate probe (before the silence timer).

1. Same setup as test 3.
2. Start detect, `synctest.Wait()`.
3. Send a valid NMEA packet (at t=0, before 1s silence timer).
4. `synctest.Wait()` -- detect processes the packet, `validPacketReceived` becomes true, `maybeSendProbe(0)` sends the probe.
5. Assert `fakeOutPort` has 1 write.
6. Close `packetCh` to finish.

### Test 5: probing success

Goal: probe response makes detection succeed with the correct config protocol.

1. Setup with `fakeConfigProtocol` (`probeOK: false`), `probeEnabled=true`.
2. Start detect, `synctest.Wait()`.
3. Send a valid NMEA packet (triggers probe), `synctest.Wait()`.
4. Set `fakeConfigProtocol.probeOK = true`, send another NMEA packet.
5. `synctest.Wait()` -- detect checks `probeSucceeded()`, finds it, returns.
6. Assert `configProt` is the fake protocol, `err == nil`.

### Test 6: socket mode

Goal: probe is sent immediately, detection validation is skipped.

1. Setup with `fakeConfigProtocol`, `probeEnabled=true`, `socket=true`.
2. Start detect. In socket mode, `maybeSendProbe(0)` is called synchronously before the select loop.
3. `synctest.Wait()`, assert `fakeOutPort` has 1 write.
4. Set `probeOK = true`, send an NMEA packet.
5. `synctest.Wait()`, receive result.
6. Assert `configProt` is the fake protocol, `err == nil`.
7. Also test `socket=true, probeEnabled=false` returns `(nil, nil)` immediately.
