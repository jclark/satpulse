## Changes in 0.3

_Not yet released_

### RTK and RTCM

- `satpulse.toml` has a new `[ntrip]` table and `[[ntrip.mountpoint]]` table array, which make `satpulsed` act as an Ntrip caster serving RTCM correction data from the receiver. Authentication is supported in conjunction with a new `[[user]]` table array. (#126)
- `satpulse.toml` has a new `[stream.pull]` table, which makes `satpulsed` act as an Ntrip client, pulling correction data from an Ntrip caster and feeding it to the receiver. A plain TCP correction source can also be used. (#221)
- `satpulse.toml` has a new `[[stream.push]]` table array, which makes `satpulsed` act as an Ntrip server, pushing receiver packet streams to a remote Ntrip caster. RTCM is the default payload, and NMEA or UBX can be selected explicitly. (#238)
- `[[stream.push]]` entries can now use `udp.address` to send receiver packet data to a UDP destination. (#320)
- `satpulse.toml` has new `msm7to4` options on Ntrip mountpoints and push entries, which make `satpulsed` convert RTCM MSM7 packets to MSM4 before forwarding them, while leaving non-MSM7 packets unchanged. (#126, #238, #288)
- The `[gps]` table in `satpulse.toml` has a new `rtcmPreferMSM7` key, which lets `satpulsed` choose whether receiver RTCM output should prefer MSM7 or MSM4 when `rtcmOutput` is enabled and both are supported. (#288)
- The device-independent GPS model now includes correction reports for RTCM correction data received by the system; these are exposed in the JSONL event log. Reports can come from correction data pulled from a network source, or from receiver-reported correction status when the receiver is configured to emit it. (#237)
- The web dashboard has an RTCM card showing correction-report data, including per-message counts and receiver-reported usage when available. (#237)
- Prometheus metrics now expose correction-report data. (#237)

### u-blox protocol-specific support

- The existing `--save` option for `satpulsetool gps`, previously used with high-level configuration, now also works with message files. New u-blox-specific message types use this so the same tag can make either a RAM-only change or a persistent change. (#272)
- The u-blox Gen 9 message file adds a broad set of tags for enabling messages. These use a new higher-level message type that works in conjunction with a new `--port` option for `satpulsetool gps`, which determines which port (`i2c`, `uart1`, `uart2`, `usb`, or `spi`) the tag enables messages on, and also supports the new `--save` behavior. (#270)
- `satpulsetool gps` has a new `--show-port` option that reports the active receiver port, such as `USB` or `UART1`, and the serial speed for UART ports. This helps choose the `--port` value for port-dependent message-file entries. (#271)
- u-blox `UBX-RXM-COR` messages are translated into the device-independent correction report, and the u-blox Gen 9 message file includes a tag to enable them. (#237)
- `satpulsed` warns when u-blox `UBX-MON-COMMS` reports transmit-buffer overflow, and the u-blox Gen 9 message file includes a tag to enable it. (#273)
- `satpulsed` logs u-blox `UBX-INF-*` messages emitted by the receiver. (#273)

### NTP support

- SatPulse now supports the NTP SHM protocol in addition to the chrony refclock SOCK protocol for sending time information to an NTP server. `satpulse.toml` has a new `[ntp.shm]` table for configuring this. (#300)

### GPS high-level configuration

- `satpulsetool gps` has new `--signal` and `--except-signal` options for controlling individual GNSS signals. `--signal` enables individual signals, in addition to the constellations enabled by `--gnss`; `--except-signal` excludes individual signals from those constellations. `--band` does not affect what signals are enabled by `--signal`. Signals are named by the constellation name followed by the signal name (`GPSL1C`, `QZSSL1S`); the constellation name is not required for Galileo and BeiDou signal names (`E5b`, `B1C`). (#97)
- `satpulsetool gps` has a new `--fixed-pos-llh` option for configuring fixed antenna position with latitude, longitude, and WGS84 ellipsoid height, instead of requiring ECEF coordinates. (#146)
- The `[gps]` table in `satpulse.toml` has a new `fixedPosLLH` key for configuring fixed antenna position with latitude, longitude, and WGS84 ellipsoid height, instead of requiring ECEF coordinates. (#147)
- `satpulsetool gps --show-receiver` now prints a `Supports:` line listing the receiver configuration features that SatPulse can use. (#203)
- `satpulsetool gps` now warns if a specified configuration option could not be applied because it is not supported by the receiver. (#203)
- Unicore receivers now support baud rate configuration with `--speed`. (#167)
- `satpulsetool gps --show-port` now works on Unicore receivers. (#167)
- `satpulsetool gps` has a new `--json` option that writes receiver information, supported configuration features, detected packet formats, and configuration properties to stdout as a single JSON object, for use by scripts and test harnesses. (#310)

### RINEX observation conversion

- `satpulsetool` has a new `convobs` command, which converts raw GNSS observation data from u-blox UBX-RXM-RAWX, Unicore OBSVM, or RTCM MSM7 into RINEX observation files for PPP post-processing. It can decimate observations and set RINEX header metadata from command-line options or a TOML header file. `convobs` also supports a new JSON Lines observation format that follows RINEX semantics to enable convenient processing with modern tooling such as `jq`; it reads and writes this format, and can also read existing RINEX files, so it can convert freely between raw packets, RINEX, and the JSON Lines format. (#296)

### Other satpulsetool improvements

- `satpulsetool` has a new `pack` command, which reads a JSONL packet log and writes selected packets as a packet byte stream corresponding to the original packet contents. It can filter by packet `tag` and `msg`, and can preserve inter-packet timing for FIFO-based replay. (#247)
- `satpulsetool` has a new `scan` command, which reads a raw GPS packet byte stream and writes a JSONL packet log that can be decoded with `satpulsetool annotate`. (#246)

### Miscellaneous

- GPS message files are now installed by packages under `/usr/share/satpulse/gpsmsg`, and by `make install` under `/usr/local/share/satpulse/gpsmsg`. The files are organized by vendor directory. (#233)
- The `satpulse@.service` has been improved so that if a USB GNSS receiver is unplugged, its `satpulse@...` service stops, and when the receiver is plugged back in, its service is automatically restarted, provided it was enabled. To take advantage of this after installing the new unit file, previously enabled instances need to be reenabled, for example with `systemctl reenable satpulse@ttyS0`. (#172)
- The packaged `satpulse@.service` unit now runs with improved systemd security hardening. (#254)
- The JSONL event log now uses a natural event shape with a `type` discriminator and a `data` payload, replacing the previous one-field-per-type record shape. The `nanos` integer field is replaced by a `mono` field holding monotonic elapsed seconds. This matches the envelope already emitted by `satpulsetool replay`. Existing event logs in the old format can be converted with the `migrate_log.go` tool in `time/internal/gpsevent`. (#277)

## Changes in 0.2

_Released 2026-05-07_

### Position and velocity

- The device-independent GPS model now includes events for geodetic/ECEF position and geodetic/ECEF velocity; these are exposed in the JSONL event log. (#220)
- The HTTP monitoring interface has a `/position` JSON endpoint.
- The web dashboard shows position and velocity information.
- Prometheus metrics include GNSS position and velocity data.
- `satpulse.toml` has a new `log.track` option, which makes `satpulsed` write a JSONL track log with one trackpoint per navigation epoch; the included `track2gpx.py` script converts this log to GPX 1.1.

### Solution quality

- The device-independent GPS model now includes an event emitted at the end of each navigation epoch, with solution-quality data that applies to the navigation solution as a whole, including fix dimensionality, correction type, accuracy, DOP, correction age, and satellite/signal information; this is exposed in the JSONL event log. (#217)
- The web dashboard shows richer GNSS solution-quality information.
- Prometheus metrics include richer GNSS solution-quality data.

### GPS high-level configuration

- SatPulse now has high-level configuration support for Unicore receivers. (#139)
- u-blox high-level configuration support now includes the ZED-X20P. (#205)
- The `[gps]` table in `satpulse.toml` and `satpulsetool gps` have expanded `vendor` handling, which restricts which packet formats are recognized and which configuration protocols are probed. (#202)
- `satpulsetool gps` has a new `--min-elev` option, which configures the receiver elevation mask. (#140)
- The `[gps]` table in `satpulse.toml` has a new `minElevation` key, which makes `satpulsed` configure the receiver elevation mask. (#140)
- `satpulsetool gps` has a new `--rtcm-base-id` option, which configures the RTCM reference station ID used in receiver-generated RTCM messages. (#106)
- The `[gps]` table in `satpulse.toml` has a new `rtcmBaseID` key, which makes `satpulsed` configure the RTCM reference station ID used in receiver-generated RTCM messages. (#106)
- `satpulsetool gps` has a new `--show-receiver` option, which explicitly enables the default behavior of detecting the GPS receiver and showing receiver information.

### Message files

- Message files are a new TOML-based way to send named receiver-specific command sequences. They work both as the main configuration mechanism for receivers without high-level configuration support, and as a way to use receiver-specific features that are outside SatPulse's high-level GPS model.
- `satpulsetool gps` has new `--msg-file`, `--tag` and `--show-tags` options for selecting message files, sending tagged command sequences, and listing available tags.
- Message files support multiple command/message forms, shared includes, deterministic tag selection, delays and wait limits, and request/response matching for supported protocols. (#200, #235, #249)

### Support for more GNSS protocols

- SatPulse now supports a broad range of GNSS receiver protocols beyond u-blox and Unicore. This support does not include the high-level, device-independent configuration interface, but it does include packet decoding, translation into device-independent GPS events, and protocol-specific message-file types with request/response correlation. (#196, #214)
- For supported receivers using these protocols, SatPulse includes message files that provide wide-ranging configuration coverage. The message tags follow device-independent naming conventions where possible, while message enablement remains protocol-specific.
- The Allystar binary protocol is supported, with message files for Allystar TAU1201 and TAU951M-P200 receivers.
- The CASIC binary protocol is supported, with message files for Zhongke ATGM332D/ATGM336H receivers and the AT632-6T-30 timing receiver.
- The Quectel PQTM NMEA-based protocol is supported, with a message file for the Quectel LG290P.
- The Airoha PAIR NMEA-based protocol is supported, with a message file for the Quectel LC29H (which uses a combination of the PQTM and PAIR protocols).
- The NovAtel OEM6/7 binary and ASCII protocol is supported in the variants used by ByNav and SinoGNSS/ComNav, with message files for ByNav M2 and SinoGNSS/ComNav K901/K902.
- The Techtotop/Taidou SDBP protocol is supported, with a message file for the Techtotop/Taidou T303-5D receiver.

### NTP support without a PHC

- `satpulsed` can provide serial timing samples to chrony or ntpd-rs without a PHC, letting the NTP server use them to number the PPS edges from a PPS refclock. This enables a complete GNSS-based NTP server to be built using SatPulse without any specialized hardware. (#77)

### PHC and PTP support

- `satpulsed` has a new PHC synchronization architecture with reset, converging and tracking modes. Prometheus metrics include the current synchronization mode. (#177, #190, #186, #187, #193)
- `satpulse.toml` has a new `[sync]` table, which lets `satpulsed` synchronization parameters be configured separately for reset, converging and tracking modes. (#179, #224)
- `satpulsetool` has a new `syncsim` command, which simulates synchronizing a PHC to a GPS receiver using the same synchronization code as `satpulsed`. It can be used to tune synchronization parameters and evaluate behavior under faults. (#173)
- The `[ptp]` table in `satpulse.toml` has new `offsetScaledLogVariance` and `allanDeviation` keys, which let `satpulsed` report clock stability to a PTP grandmaster. (#189)

### Other satpulsetool improvements

- `satpulsetool gps` has a new `-f`/`--config-file` option, which reads the serial device path and speed from a `satpulse.toml`-compatible `[serial]` table.
- `satpulsetool gps` has a new `--packet-log` option, which writes GPS packet traffic to a JSONL packet log.
- `satpulsetool gps` has a new `--capture` option, which can be used with `--packet-log` to run in a packet-capture-only mode.
- `satpulsetool decode` decodes a single GPS packet from hex or ASCII data and prints decoded JSON.
- `satpulsetool annotate` reads a JSONL packet log and adds decoded packet fields, such as headers, payloads and configuration data.
- `satpulsetool replay` reads a JSONL packet log, runs incoming packets through the GPS protocol processing pipeline, and emits device-independent GPS events as JSONL.
- `satpulsetool ntrip` fetches data from an Ntrip caster mountpoint and writes a JSONL packet log by default, or raw bytes with `--bin`.

### Miscellaneous

- `satpulsetool` can now be built for Windows using the `win-build.ps1` script. It supports the `gps`, `decode`, `annotate`, `replay` and `ntrip` subcommands.
- `satpulsed` now uses distinct exit statuses for usage errors, permission errors and configuration-file errors, so systemd can avoid restarting the daemon for failures that require user action. (#171)

---

0.1 was released on 2026-01-23.
