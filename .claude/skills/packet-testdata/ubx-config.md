# u-blox specific capture details

## Product categories

u-blox receivers have product categories that affect which messages high-level config enables:

- **HPG** (High Precision GNSS, e.g., ZED-F9P): Uses NavHPPosLLH/NavHPPosECEF instead of NavPosLLH/NavPosECEF.
- **FTS** (Frequency and Time Standard): Uses TimTos instead of TimTP.
- **Standard** (e.g., NEO-M8): Uses NavPosLLH/NavPosECEF directly.

## How `--pvt-out` maps to UBX messages

The `pvt()` function in `gps/internal/ubx/ubxcfgmsg.go` determines which UBX messages get enabled. Key rules:

- `pos` on HPG => NavHPPosLLH (not NavPosLLH)
- `pos,ecef` on HPG => NavHPPosECEF (consumes ecef flag)
- `vel` => NavVelNED
- `vel,ecef` => NavVelECEF (but on HPG, `pos,vel,ecef` gives NavHPPosECEF + NavVelNED because `pos` consumes ecef first)
- `time` (without `tai`) => NavTimeUTC
- `time,tai` => NavTimeGPS
- `tp` => TimTP (or TimTos on FTS); `tp,after` adds `time` flag
- `qual` => NavDOP (also triggers NavPVT if combined with any of pos/vel/time)
- `epoch` => NavEOE
- `leap` => NavTimeLS
- `survey` + `--survey` => NavSvin (HPG) or TimSvin (timing receivers)
- When >= 2 of remaining {pos, vel, time} are set, or `qual` is set: NavPVT is enabled and subsumes them
- `off` turns off messages that would otherwise be left at their current rate

Understanding the ecef flag consumption is critical: on HPG, `pos` with `ecef` consumes the ecef flag before `vel` can use it. To get NavVelECEF on HPG, use `vel,ecef` without `pos`.

## Messages not reachable via high-level config

These need low-level message files (CFG-VALSET on Gen9+ receivers):

- NavTimeBDS, NavTimeGLO, NavTimeGal, NavClock (no high-level flag)
- NavPosLLH, NavPosECEF (on HPG, high-level always uses HP variants)
- NavSVInfo (legacy, replaced by NavSat on protocol >= 15)
- TimSvin (timing/FTS receivers, not HPG)
- TimTos (FTS only)

## Low-level message enable via CFG-VALSET (Gen9+ only)

Gen9+ u-blox receivers (protocol >= 27) use CFG-VALSET (class 0x06, id 0x8A) to configure message output rates. The MSGOUT keys are port-specific:

- Port offsets: I2C=0, UART1=1, UART2=2, USB=3, SPI=4
- Key formula: `0x20910000 + KeyM + port_offset`
- KeyM values are defined in `gps/lib/ubxcfgval/msgkey.go`

Message file tags for Gen9+ messages are in `configs/gpsmsg/ubx9.toml` (which includes `ubx.toml` for generic u-blox commands). Tag naming convention: `ubx-<msg>-usb` (enable) and `ubx-<msg>-usb-off` (disable). For UART connections, equivalent `-uart1` tags would be needed.

CFG-VALSET payload: version=0 (U1), layers=1 for RAM (U1), transaction=0 (U2), then key (U4) + value (U1).

## Older u-blox receivers (pre-Gen9)

Pre-Gen9 receivers (protocol < 27) use CFG-MSG (class 0x06, id 0x01) to set message rates. The payload is: message class (U1), message id (U1), rate for each port (6x U1). Message file tags for these would need different payloads.

Pre-Gen9 differences:
- No NavSig (protocol < 27)
- No NavEOE, NavTimeLS (protocol < 18)
- NavSVInfo instead of NavSat (protocol < 15)

## Protocol version checks

Always check protocol version from `--show-receiver` before planning captures. Messages that don't exist on the receiver's protocol version should be skipped, not attempted.

## Completed captures

See `plan/archive/packet-capture-zed-f9p.md` for a complete worked example of capturing from a ZED-F9P (HPG, protocol 27.50, USB).
