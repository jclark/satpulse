# scan.Scanner: caller-supplied packet formats

## Context

`scan.New()` hardcodes `gpsreg.PacketFormats`, making `scan` depend on `gpsreg`. This is a layering violation: `scan` is a low-level packet framing package and should not know about the registry of all known formats. The caller should supply the formats it needs.

This is a prerequisite for the RTK rover feature, which needs a scanner that recognizes only RTCM packets on a TCP stream.

Target branch: `master`.

## Change

Change `scan.New` signature from:

```go
func New(r io.Reader, bufSize int) *Scanner
```

to:

```go
func New(r io.Reader, bufSize int, pktFormats []gpsprot.PacketFormat) *Scanner
```

Remove the `gpsreg` import from `scan/scan.go`.

## Files to modify

### `gps/scan/scan.go`

- Add `pktFormats` parameter to `New`.
- Assign `s.pktFormats = pktFormats` instead of `s.pktFormats = gpsreg.PacketFormats`.
- Remove the `gpsreg` import.

### `gps/app/gpsio/conn.go` — `Scan()`

Add a `pktFormats []gpsprot.PacketFormat` parameter to `gpsio.Scan` and pass it through to `scan.New`.

### Callers of `gpsio.Scan` (pass `gpsreg.PacketFormats`)

- `time/app/daemon/daemon.go`
- `internal/gpscmd/gpscmd.go`

Note: `desktop/app.go` is in a separate repo (`satpulse-desktop`) and will be updated there.

### `gps/gpsdecode/gpsdecode.go` — `Decode()`

Already receives `pktFormats []gpsprot.PacketFormat` as a parameter but doesn't pass it to `scan.New()`. Fix to pass it through. Remove the now-unused `gpsreg` import if applicable.

### `internal/gpscmd/response_test.go`

Add `gpsreg.PacketFormats` as the third argument to `scan.New`.

### `gps/scan/*_test.go` — change to `package scan_test`

Change all test files from `package scan` to `package scan_test`. This cleanly separates the test dependencies (protocol-specific packages) from the scan package itself, which after this change has no protocol dependencies.

Each test should explicitly specify which packet formats the scanner uses. This makes tests robust when new packet formats are added (currently they implicitly get all formats via `gpsreg`). The formats a test uses determine what it's actually testing: a UBX test with only UBX format active tests different scanner paths than one with all formats active.

Specific format choices per test file:

- **`nmea_test.go`**: Use `nmea.PacketFormat` only. Tests NMEA packet recognition and mixed valid/invalid data. Using only NMEA format is sufficient since the tests verify NMEA-specific parsing.

- **`ubx_test.go`**: Use `gpsreg.PacketFormats` (all formats). The `randomInvalidPacket` helper avoids all known sync bytes, so these tests exercise the scanner with the full format set. The `packetSyncBytes` list should move to a test helper since it enumerates all formats.

- **`rtcm_test.go`**: Use `rtcm.PacketFormat` only. Tests RTCM-specific scanning.

- **`novatel_test.go`**: Use `nov.BinPacketFormat` only. Tests NovAtel-specific scanning. Some subtests already specify `pktFormats` explicitly.

- **`unicore_test.go`**: Use `unc.BinPacketFormat` only. Tests Unicore-specific scanning. Some subtests already specify `pktFormats` explicitly.

- **`scan_test.go`**: Use `nmea.PacketFormat` only. The `TestInvalidBytesBatching` test uses NMEA data and only checks for NMEA recognition.

## Verification

```
go test ./...
```
