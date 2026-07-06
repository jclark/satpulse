# mosaic-G5 limitations

How device-independent configuration is realized on the Septentrio mosaic-G5, relative to perfect realization of the full model (`SEMANTICS.md`). Measured on firmware 1.1.0 (GNSS firmware 2026.01.2, USB CDC connection `USB1`, serial 0100019577). The characterization ran against an integration build carrying the Septentrio configurator (#341) and the SBF message conversion (#340); the disruptive sweep (saves, resets, factory reset, speed) is included. Baseline: `baselines/mosaic-G5-P3-1.1.0.json`.

## Session preconditions

The receiver is USB-connected, so runs need no speed discovery. A reproducible characterization starts from a message state the tool can reconstruct: SBF Stream1 = `PVTGeodetic` at 1 Hz, NMEA Stream1 empty (`satpulsetool gps --binary` reproduces this after the probes; a richer as-found stream cannot be rebuilt from observation and reports an honest restore failure). The state is established from factory defaults by `setup/mosaic-g5.sh`. Resets re-enumerate USB (~1.5 s, stable ttyACM numbering; `/dev/serial/by-id/usb-Septentrio_Septentrio_USB_Device_<serial>-if00` is the stable handle) and the command line answers again ~7 s after a soft reset and ~25 s after a hard one.

## Findings

- Identity (`--show-receiver` hardware "mosaic-G5 P3", firmware "1.1.0") is fetched from the Identification internal file (`lstInternalFile, Identification`), the only ASCII carrier of the firmware version: a read-only lst command, so identity never changes the receiver configuration. A ReceiverSetup SBF block is used instead whenever one arrives. (The receiver's one-shot block fetch, `exeSBFOnce`, delivers to its own connection only when the block is enabled on a stream bound to that connection - verified repeatedly, including with a raw capture - so it is not used.)
- Antenna cable delay: the receiver refuses out-of-range values rather than clamping, so satpulsetool clamps requests to the receiver's documented +-10000 ns before the wire; in-range requests realize exactly (0.001 ns resolution).
- Fixed position: latitude/longitude quantized to 1e-9 deg, ECEF coordinates and height to 0.1 mm - the receiver's own readback resolutions.
- Fixed-position accuracy has no command carrier; the backend does not declare `fixedPosAcc` support, and every requested accuracy is achieved as 0 (the bounded expectation).
- RTCM output: enabling MSM4/MSM7/ARP is acked with the expanded message list, but nothing is emitted in rover mode - RTCM emission additionally requires a static (base) positioning mode with a reference position. Recorded as missing emissions in the characterization, matching the documented dependency.
- Save granularity is a single group: `exeCopyConfigFile, Current, Boot` persists the whole configuration, so `--save` is indistinguishable from `--save-all` (`saveGranularity: singleGroup`).
- Baud rate is not applicable on the USB connection (reads back 0; sets are no-ops), following the ubx USB model. `setCOMSettings` affects only the physical COM ports, which this installation cannot reach.
- Signals: the discovered supported set matches the receiver's `getReceiverCapabilities` list through the coarse signal table; the receiver-only signals (GALE5 AltBOC, GLOL2P, QZSL1CB) have no device-independent name and are preserved as found by every signal set. The SBAS (GEO) signals cannot appear in the PVT signal-usage list (the receiver refuses them there); they ride the tracking and NavData lists only.
- Survey: no parameterized/terminating/observable survey operation exists; `--survey` semantics are shown as absence (`setPVTMode, Static, , auto` is an auto-computed reference surfaced as static mode without a fixed position).

## After a disruptive sweep

The save probes write the Boot configuration file, and the sweep's recovery does not return Boot to its pre-run content. On an installation whose Boot is factory-default, finish a disruptive session with `exeCopyConfigFile, RxDefault, Boot` (verify with `lstConfigFile, Boot` reading "Equal to RxDefault!") before re-applying the as-found running configuration.

## Known observation nondeterminism: leap

Leap-second information rides the GPSUtc/GALUtc/BDSUtc blocks, whose OnChange schedule is the UTC-parameter renewal (minutes-scale; there is no dump-on-enable - verified: 10 s after enabling all three, only one GALUtc had arrived). A 4-second observation window therefore catches leap most but not all of the time: four of five characterization sweeps observed it, one reported `pvtOut: {missing: [leap]}`. The committed baseline carries the majority case (leap observed); a baseline diff showing only that entry is this artifact, not a behavior change. The deterministic fix is decode work on the message-conversion side (the 1 Hz ReceiverTime block carries the current leap count in `DeltaLS`, unconverted today), tracked as follow-up for the SBF conversion (#340); the configurator would then add ReceiverTime to the leap block list.

## Defect history on this receiver

Characterization found three satpulsetool defects, all fixed during the bring-up: SBAS signals were placed in the PVT usage list (refused mid-realization), an explicit output disable could leave shared blocks enabled through untouched-class preservation, and an empty SBF output selection was sent as an empty argument, which the omitted-argument rule turns into keep-current. The committed baseline reflects the fixed behavior.

## Not yet covered

Physical power-cycle persistence (USB replug) needs the owner; `exeResetReceiver, Hard` is the documented and observed power-off/on-like oracle and behaves accordingly. The standalone `factoryReset` command (mark-for-reset-at-power-cycle) is deliberately unused and untested: an unconsumed mark would fire at a later physical power cycle. The PPS pin was not observed electrically (no wiring); all time-pulse findings are register semantics.
