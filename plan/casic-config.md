# CASIC configurator

Implement `gpsprot.ConfigProtocol` and `gpsprot.Configurator` for CASIC receivers, enabling `satpulsetool gps` and `satpulsed` to configure CASIC receivers (probe, enable messages, set time pulse, time mode, signal selection).

Follows the existing UBX configurator architecture.

## Stage 1: Configuration message definitions (casic/bin)

Add configuration and acknowledgment message types to `casic/bin/`.

**Files to Create:**

1. **`internal/casic/bin/ack.go`** - ACK/NAK messages
   - `AckAck` (0x05 0x01)
   - `AckNak` (0x05 0x00)

2. **`internal/casic/bin/cfg.go`** - Configuration messages
   - `CfgPrt` (0x06 0x00) - Port configuration
   - `CfgMsg` (0x06 0x01) - Message rate configuration
   - `CfgTP` (0x06 0x03) - Time pulse configuration
   - `CfgTMode` (0x06 0x06) - Time mode configuration
   - `CfgNMEA` (0x4E 0x*) - NMEA message control

**Functionality:** Binary definitions for configuration commands.

---

## Stage 2: Minimal configurator (probing + PVTMsg + NMEAMsg)

Implement `gpsprot.ConfigProtocol` and `gpsprot.Configurator` with receiver detection and message output configuration.

**Files to Create:**
- `internal/casic/casiccfg.go` - ConfigProtocol implementation

**Implement:**
- `ProbePacket()` / `ProbeOK()` for receiver detection (e.g., poll MON-VER or similar)
- `ConfigOptions.PVTMsg` support (enable time-related messages)
- `ConfigOptions.NMEAMsg` support (enable/disable NMEA messages)
- ACK/NAK handling for configuration responses

**Files to Modify:**
- `internal/gpsreg/reg.go` - Add to `CreateConfigProtocols()`

**Functionality:**
- `satpulsetool gps --binary --nmea --nmea-out --pvt-out` works with CASIC receivers
- `satpulsed` with `config=true` works minimally: can take a factory-default CASIC receiver, detect it, and configure it to emit the binary messages needed for time sync

---

## Stage 3: TimePulse configuration

Extend Configurator for time pulse settings.

**Implement:**
- `ConfigProps.TimePulse` and `ConfigProps.TimeGNSS` support via `CfgTP` message
- Width, period, polarity, alignment settings

**Functionality:** `satpulsetool gps --pps --time-gnss` works.

---

## Stage 4: TimeMode configuration

Extend Configurator for time mode settings.

**Implement:**
- `ConfigProps.Mode` support via `CfgTMode` message
- Static/mobile mode
- Survey-in mode
- Fixed position mode

**Functionality:**
- `satpulsetool gps --survey --fixed-pos-ecef --mobile` works
- `satpulse.toml` time mode options work: `mobile`, `surveyTime`, `fixedPosECEF`, `fixedPosAcc`

---

## Stage 5: SignalsEnabled configuration

Extend Configurator for GNSS signal selection.

**Implement:**
- `ConfigProps.SignalsEnabled` support
- Enable/disable GNSS constellations

**Functionality:** `satpulsetool gps --gnss --band` works.

---

## Struct patterns

### Fixed-size message

```go
type AckAck struct {
    ClsID byte
    MsgID byte
    _     [2]byte // padding to 4 bytes
}

func (m *AckAck) ID() MsgID { return AckAckID }
```

## Known errata

From `../casictool/spec/errata.md`:

1. **Checksum byte order**: Spec says `(class << 24) + (id << 16)`, but receivers use `(id << 24) + (class << 16)`
2. **MON-VER support**: Some receivers respond with ACK-NAK instead of version info
3. **CFG-MSG query**: Empty payload returns all message rates, not just one
4. **CFG-TMODE mode field**: Upper 2 bytes of U4 mode field contain unknown values; parse as U2 mode + U2 reserved

## Reference materials

- Spec: `../casictool/spec/casic.md`, `casic1.md`, `casic2.md`
- Errata: `../casictool/spec/errata.md`
- Python prototype: `../casictool/casic.py`
- UBX reference: `internal/ubx/`, `internal/ubx/bin/`
- CASIC packet processing: `internal/casic/`
