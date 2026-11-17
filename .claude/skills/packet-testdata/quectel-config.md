# Quectel LG290P specific capture details

This file covers LG290P receivers, which are configured with PQTM sentences
from `configs/gpsmsg/quectel/lg290p.toml`. The set is in
`gps/testdata/packets/quectel/LG290P/`. The captures there were made on
firmware LG290P03AANR02A01S at 460800 baud.

## Safety

Some command sequences hang the receiver, and a hung LG290P needs a power
cycle. A reset command does not recover it.

- Do not send PQTMGNSSSTOP or PQTMGNSSSTART.
- Before commit 483846f2 (term: never restore echo on Unix), every open and
  close of the port by satpulsetool echoed the receiver's own output back to
  it. With RTCM output running, the echoed partial RTCM frames made the
  receiver swallow the commands that followed, and the receiver once stopped
  answering entirely until it was power cycled. With the fix, commands sent
  while RTCM output runs are answered. The first open after the USB device
  appears still echoes.
- Captures taken before the fix can begin with a reply such as
  `$PQTMEOE,ERROR,3`: the receiver rejecting its own echoed sentence. Keep
  such captures as they are; the decoder ignores those replies.

## Resetting between captures

Do not reload or restart between captures. Start each capture's configuration
step with `msg-all-off` (PQTMCLRMSG), which turns off every output on the
port. Then enable the capture's messages in the same `satpulsetool gps -m`
run. Message rate changes take effect at once.

## Fix rate

The default fix rate is 10 Hz. The `fix-rate-*` tags are acknowledged and
read back, but the output rate does not change without a save and restart. So
the captures are at 10 Hz, with no rate suffix. For 1 Hz output, set each
message's rate to 10 (one message every 10 fixes), as in `-1hz` captures.
PQTMEOE rejects rate 10 (invalid parameters), so `-1hz` captures that include
it keep it at rate 1: an EOE ends every 100 ms epoch, most of them empty.

## Messages

- PQTMSVINSTATUS is rejected in rover mode. Base mode takes effect only after
  `save` and `reset`; then enable PQTMSVINSTATUS and a time message (in base
  mode NMEA is off and the fix rate is 1 Hz). `survey` was captured that way
  with a 60 s survey-in (PQTMCFGSVIN,W,1,60,0,0,0,0).
- PQTMANTENNASTATUS, PQTMTAR, HDT and THS are rejected; the spec lists them
  for other modules.
- PQTMTXT is enabled by default but was not output during any capture.
- PQTMPPPNAV is output with PPP disabled. It has SolType and Datumid but no
  position.
- PQTMRTCMIS is output only while RTCM is being received. Feed RTCM from a
  satpulsed test instance with `[stream.pull]` (for example the NTRIP caster
  `serpa.lan:2101`, mountpoint `BKK`), and keep only the incoming packets
  (`jq -c 'select(.out != true)'`). No PQTMRTCMIS is output for RTCM 1230.

## RTCM output

RTCM output works in rover mode. MSM for all six systems, 1005, 1033 and 1230
are output at the fix rate, and ephemerides when they update. The MSM type
(`rtcm-msm7`) takes effect only after `save` and `reset`. `mode-base` sets
the MSM type back to 4; `rtcm-msm7` sent in base mode, then `save` and
`reset`, gives MSM7 in base mode. `rtcm-msm7` was captured in rover mode after save
and reset; `survey` has base-mode MSM4 and 1005.

RTCM ephemerides need both their switches (`rtcm-eph`: PQTMCFGMSGRATE rate 1,
the only rate allowed) and the timing in PQTMCFGRTCM EPH_Mode, which takes
effect only after `save` and `reset`. With the factory EPH_Mode 1 each
ephemeris is output only when it updates, a few a minute. With EPH_Mode 2
and a 30 s interval (PQTMCFGRTCM,W,4,0,-90.0,07,06,2,30), every 30 s the
receiver outputs the ephemeris of every tracked satellite (62-63 messages for
six systems), plus single messages on update in between. After `rtcm-eph`
switched them on, the first full set came 7 s later. `rtcm-eph` was
captured that way, 120 s with RMC, then the receiver was factory reset.

## PPP

PPP-B2b takes 20 to 30 minutes to converge, so a B2b capture needs a long
passive capture after `ppp-b2b`.

## Factory, cold start and restoring

These change NVM, so ask first, and record the receiver's configuration
beforehand (every PQTMCFG read) so it can be restored.

- `factory`: `factory-reset`, then capture.
- `coldstart`: enable the messages, `save`, `cold-start`, then capture
  120 s immediately.
- Afterwards `factory-reset`, then restore anything that differed from factory
  (on roquefort only the PPS mode, 2) and `save`.
