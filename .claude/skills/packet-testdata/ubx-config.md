# u-blox specific capture details

## Product categories

u-blox receivers have product categories that affect which messages high-level config enables:

- **HPG** (High Precision GNSS, e.g., ZED-F9P): Uses NavHPPosLLH/NavHPPosECEF instead of NavPosLLH/NavPosECEF.
- **TIM** (Timing, e.g., LEA-M8T): Uses TimSvin for survey (not NavSvin). Message selection is otherwise the same as standard.
- **FTS** (Frequency and Time Standard, e.g., LEA-M8F): Uses TimTos instead of TimTP. TimTos is NOT available on TIM receivers.
- **Standard** (e.g., NEO-M8): Uses NavPosLLH/NavPosECEF directly.

The product category is shown by `--show-receiver` in the firmware field (e.g., "TIM 1.10 PROTVER 22.00" or "HPG 1.51 PROTVER 27.50"). It is also recorded in the HW.toml `firmware` field. The `pvt()` function in `ubxcfgmsg.go` only checks for "HPG" and "FTS" -- "TIM" is treated like standard for message selection.

**CRITICAL**: Do not confuse TIM and FTS. They are different product categories with different behavior. TIM uses TimTP (like standard); FTS uses TimTos.

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

## How `--raw-out` maps to UBX messages

From the `raw()` function in `gps/internal/ubx/ubxcfgmsg.go`:

- `obs` => RXM-RAWX on modern receivers, RXM-RAW on legacy receivers.
- `nav` => RXM-SFRBX (modern) or RXM-SFRB (legacy).

The choice between modern and legacy IDs is driven by `Version.rawLevel()`. Receivers without raw support (`rawLevel == 0`) reject `--raw-out`. Among receivers in the test collection: ZED-F9P, ZED-F9T, ZED-X20P, and LEA-M8T use the modern IDs; LEA-6T uses the legacy IDs.

Suggested capture names: `raw-obs.jsonl`, `raw-nav.jsonl`, `raw-obs-nav.jsonl` (with the usual `-NNNN` baud suffix if not at the default rate). Combine with a minimal time set so the capture has at least one time message:

```
--binary --pvt-out tp,after,tai,off --raw-out obs,nav
```

## Raw cross-format capture

On u-blox receivers that support both RXM-RAWX and RTCM MSM7 output (currently F9P and later), capture them together in a binary-only trace with no time message. F9P emits RTCM only when `CFG-TMODE-MODE` is set to Fixed or Survey-In; in the default Disabled mode RTCM messages are configured but never sent. Pass `--fixed-pos-ecef <X,Y,Z> --fixed-pos-acc <m>` with the antenna's known ECEF coordinates:

```
satpulsetool gps -d <device> -s <baud> --binary --pvt-out off --raw-out obs --rtcm-out MSM7 \
  --fixed-pos-ecef <X,Y,Z> --fixed-pos-acc <m> \
  --packet-log raw-cross.jsonl --capture 30
```

`--pvt-out off` is essential -- it disables the daemon time-message set that high-level config would otherwise enable. The MSM7 messages and RXM-RAWX both carry their own GPS time; the cross-format test compares those embedded times.

The F9P captures GPS, GLONASS, Galileo, and BeiDou MSM7 (1077, 1087, 1097, 1127) and the 1230 GLONASS code-phase bias. It does not emit QZSS MSM7 (1117). RTCM 1005 is also enabled by `--rtcm-out MSM7` if combined with `,ARP`, but is not required for cross-format testing.

Older u-blox receivers (LEA-M8T, LEA-6T) do not emit MSM7 and are skipped for this capture.

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

Message file tags for Gen9+ messages are in `configs/gpsmsg/u-blox/gen9.toml` (which includes `ubx.toml` for generic u-blox commands). The tags are now port-independent: `ubx-<msg>` (enable) and `ubx-<msg>-off` (disable), e.g. `ubx-nav-timebds` / `ubx-nav-timebds-off`. These resolve to `[[ubxvalport]]` entries, and the active port is supplied at the command line with `--port <i2c|uart1|uart2|usb|spi>` -- the port offset above is applied by the tool, not baked into the tag. Determine the active port with `--show-port` (the `Port:` line). On an evaluation kit such as the EVK-M101, the USB connector is a USB-serial adapter wired to the module's UART1, so use `--port uart1`, not `usb`.

CFG-VALSET payload: version=0 (U1), layers=1 for RAM (U1), transaction=0 (U2), then key (U4) + value (U1). `--save` with these tags persists to `RAM|BBR|Flash` instead of just RAM.

## Older u-blox receivers (pre-Gen9)

Pre-Gen9 receivers (protocol < 27) use CFG-MSG (class 0x06, id 0x01) to set message rates. The payload is: message class (U1), message id (U1), rate for each port (6x U1). Message file tags for these are in `configs/gpsmsg/u-blox/gen8.toml` (CFG-MSG rate tags), as distinct from the CFG-VALSET tags in `gen9.toml`.

Pre-Gen9 differences:
- No NavSig (protocol < 27)
- No NavEOE, NavTimeLS (protocol < 18)
- NavSVInfo instead of NavSat (protocol < 15)

## Protocol version checks

Always check protocol version from `--show-receiver` before planning captures. Messages that don't exist on the receiver's protocol version should be skipped, not attempted.

## Troubleshooting

### Serial overload at low baud rates

At 9600 baud, enabling extra UBX messages on top of default NMEA via message file tags can overload the serial link. When this happens, probe commands time out ("no response to configuration probe message").

To avoid overload: if you have enabled extra messages at a higher baud rate, disable them (or reload) *before* changing to a lower baud rate.

To recover from an overloaded link, use the message file `reload` tag (which doesn't need probing):

```
satpulsetool gps -d <device> -s 9600 --vendor u-blox -m configs/gpsmsg/u-blox/gen8.toml -t reload --capture 3
```

### Per-constellation capture verification

After each per-constellation time capture, immediately verify the TIM-TP `refInfo` field shows the expected GNSS:

```
satpulsetool annotate <file> | jq -rc 'select(.msg=="TIM-TP") | (.payload.refInfo % 16)' | sort | uniq -c
```

`refInfo` encodes the GNSS ID in the lower nibble: 0=GPS, 1=GLONASS, 2=BeiDou, 3=Galileo. If it shows the wrong GNSS, the constellation may not have been acquired yet. Allow 10-15 seconds after a constellation change before starting the capture (GLONASS, being FDMA, can take notably longer to lock).

## Completed captures

See `plan/archive/packet-capture-zed-f9p.md` for a complete worked example of capturing from a ZED-F9P (HPG, protocol 27.50, USB).
