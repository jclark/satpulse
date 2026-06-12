# RTCM ephemeris support (#282)

Adds decode/encode of RTCM ephemeris messages to `rtcmbin`, a new
`RTCMMsgEph` config flag, Unicore driver support for enabling
per-constellation ephemeris output, and replay-on-connect in the
NTRIP caster so rovers cold-start without waiting for the broadcast
cycle.

## Messages in scope

| Msg  | Constellation | Notes |
|------|---------------|-------|
| 1019 | GPS           | LNAV |
| 1020 | GLONASS       | |
| 1041 | NavIC/IRNSS   | |
| 1042 | BeiDou        | D1/D2 |
| 1044 | QZSS          | |
| 1045 | Galileo       | F/NAV (E5a) |
| 1046 | Galileo       | I/NAV (E1-B, E5b) |

All seven are decoded (for `satpulsetool decode` and for upstream
NTRIP re-casting).  Transmit support is narrower:

- Unicore UM980 series accepts `RTCM1044` (verified empirically;
  not documented in unicore1.13.md) but rejects `RTCM1041` with
  `PARSING FAILED NO MATCHING FUNC`.
- u-blox emits none of the seven.

## Receiver emission behavior

Verified empirically on UM982 at 115200 baud:

- At `RTCM1019 1` (1 Hz), Unicore emits one satellite's ephemeris per
  epoch, round-robin across tracked sats. Each message is fixed-size
  (~50-70 bytes).
- Per-epoch emission order is deterministic: NMEA, then Unicore
  binary (RECTIME etc.), then RTCM in the tail. Adding ephemeris
  RTCM messages extends the tail but does not affect the offset of
  RECTIME within the epoch (RECTIME is emitted before any RTCM).
- Total bandwidth at 1 Hz across all five ephemeris types: ~325 B/s.
  No burst risk.

u-blox (F9P, X20 HPG) cannot emit any of these messages.

## 1. Add ephemeris decode/encode to `rtcmbin`

Add struct definitions in `gps/lib/rtcmbin/mt.go` for each ephemeris
message using the existing `bitsenc` struct-tag pattern.  Bit layouts
are in RTCM 10403.2 sections 3.5.5-3.5.12.

Each struct embeds `MsgHdr`, starts with the satellite ID field,
then the full ephemeris parameter set.  No `SizeSlices` needed -
ephemeris messages are fixed-length.

Add `MT1019`, `MT1020`, `MT1041`, `MT1042`, `MT1044`, `MT1045`,
`MT1046` cases to `ParseMsg` in `rtcmbin.go` and the corresponding
serialize path.

Add an `EphSatID` accessor interface (or a switch in the caster) so
the caster can get the satellite ID from any ephemeris `Msg` without
type-asserting to each concrete type.

Test fixtures: use real captures from a UM982 (stored under
`gps/lib/rtcmbin/testdata/`).  The capture from the design
experiment (`/tmp/rtcm-timing.jsonl`) has one sample of each message
type and should be imported.

Verify via `satpulsetool decode --bin <hex>` that the JSON output
contains all fields with plausible values, and that round-trip
decode->encode produces identical bytes.

## 2. Add `RTCMMsgEph` flag to `gpsprot`

In `gps/gpsprot/configtarget.go`:

- Add `RTCMMsgEph` as the next bit in `RTCMMsgFlags`.
- Include it in `RTCMMsgAuto` (which becomes
  `MSM4 | ARP | Eph | Lax`).
- Add it to `RTCMMsgAny`.
- Extend the `--rtcm-out` keyword parser in
  `internal/gpscmd/gpsflags.go` to accept `eph`.

TOML surface (`rtcmOutput = true`) is unchanged - it already maps
to `RTCMMsgAuto`, which now includes Eph.

## 3. u-blox: handle Eph

u-blox cannot emit ephemerides.  Follow the pattern already
established in `gps/internal/ubx/ubxcfgmsg.go` for unsupported
MSM types:

- With `RTCMMsgLax` set: silently skip `RTCMMsgEph`, emit no
  ephemeris-related commands.
- Without `RTCMMsgLax`: return an error
  (`"RTCM ephemeris output not supported by this model"`).

This matches the existing behavior for MSM4-on-F9T and keeps the
overall rule simple: "config is best-effort under Lax; strict
otherwise."

In practice users hit the Lax path almost always, because
`RTCMMsgAuto` includes Lax.  The non-Lax path is only reached when
a user explicitly requests `--rtcm-out eph` (or some combination
without `lax`) on a u-blox target -- in which case a clear error is
the right response.

Add test cases:

- `RTCMMsgAuto` against a u-blox target produces the same output as
  `RTCMMsgMSM4 | RTCMMsgARP | RTCMMsgLax`.
- `RTCMMsgEph` alone against a u-blox target returns the expected
  error.

## 4. Unicore: emit per enabled constellation

In `gps/internal/unc/cfgopts.go`, extend
`generateRTCMMsgCommands` to emit the appropriate ephemeris
commands when `RTCMMsgEph` is set, gated on enabled GNSS:

```go
eph := flags&gpsprot.RTCMMsgEph != 0
type gnssEph struct {
    gnss gpsprot.GNSS
    ids  []string
}
gnssEphs := []gnssEph{
    {gpsprot.GPS, []string{"RTCM1019"}},
    {gpsprot.GLO, []string{"RTCM1020"}},
    {gpsprot.BDS, []string{"RTCM1042"}},
    {gpsprot.GAL, []string{"RTCM1045", "RTCM1046"}},
    {gpsprot.QZSS, []string{"RTCM1044"}},
}
for _, g := range gnssEphs {
    if enabledGNSS.Contains(g.gnss) {
        for _, id := range g.ids {
            appendMsgCmd(&cmds, id, eph)
        }
    }
}
```

NavIC is not in the list: `RTCM1041` is rejected by UM982 firmware
Build13495.  Under `Lax` we silently skip, consistent with the
general handling of unsupported messages.

Rate: 1 Hz (`RTCM1019 1`, etc.).  This matches Unicore's natural
round-robin: one sat per constellation per second, full cycle across
all tracked sats in ~30 s.

Update the testdata in `internal/gpscmd/testdata/unicore/` to cover
the `RTCMMsgEph` case.

## 5. Caster: replay ephemerides on connect

Depends on `plan/ntrip-caster.md`.

### Cache

Add an ephemeris cache in `gps/app/ntrip`:

```go
type ephCache struct {
    mu    sync.Mutex
    ephs  map[ephKey][]byte // raw RTCM packet bytes
}

type ephKey struct {
    MsgType rtcmbin.MsgType // 1019, 1020, 1042, 1045, 1046
    SatID   uint8
}
```

The cache is populated by the same bcast subscription that feeds
clients.  For each incoming RTCM packet whose message type is an
ephemeris type, parse enough to extract the satellite ID (via the
new `rtcmbin` helper) and store the raw packet bytes keyed by
`(msg type, sat ID)`.  Newer emissions overwrite same-key entries.

No explicit TTL: stale ephemerides are rejected by the rover's
IODE check.  Entries are simply overwritten as newer ones arrive.
On satpulsed restart the cache starts empty and refills over ~30 s.

### Replay

On client connect, after the NTRIP response headers are written
and before the live bcast loop starts, snapshot the cache under
its lock, release the lock, and write each cached packet to the
client.  Then subscribe to bcast and enter the normal stream
loop.

No per-mountpoint replay knob - ephemeris replay is always on.
At ~150 satellites total * ~64 bytes, worst-case replay is ~10 KB,
trivial.

### No interaction with MSM7→MSM4 conversion

Ephemeris messages pass through unchanged.  The `msm7to4`
conversion in the caster applies only to MSM7 observation
messages; ephemerides are forwarded as-is from cache.

## Testing

- Unit tests for each ephemeris message struct: bit layout (compare
  against hand-decoded reference values from the UM982 captures)
  and round-trip decode->encode.
- Unit test for the caster's `ephCache`: feed a stream of packets,
  verify overwrite-on-same-key, verify snapshot returns current
  contents.
- Integration test (local TCP listener + synthetic bcast): verify
  that a client connecting after some ephemerides have been cached
  receives them in the initial burst before live packets.

## Implementation order

1. rtcmbin structs + ParseMsg/SerializeMsg cases + tests.
2. Satellite ID accessor.
3. `RTCMMsgEph` flag in gpsprot + CLI keyword.
4. u-blox Lax verification.
5. Unicore cfgopts + tests.
6. Caster ephCache + replay (after ntrip-caster.md lands).
