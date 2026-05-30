# convobs golden test fixtures

These files back `TestGoldenFiles` in `convobs_test.go`. Each test case
converts a raw observation input with `convobs` and compares the result,
semantically, against a golden RINEX observation file produced by RTKLIB
Explorer's `convbin`. The comparison ignores metadata that depends on run
time, and a small whitelist of signals where SatPulse and `convbin` disagree
in ways the protocol specs resolve in SatPulse's favour (documented in the
test).

The golden files are therefore an independent reference (RTKLIB Explorer),
not SatPulse self-output. Regenerate them only when the reference itself
changes, and review any resulting observation differences before committing.

## Tools

- `convbin` is the RTKLIB Explorer build, not the system `convbin`. Build it
  from `~/rtklib-ex` so it matches the `~/rtklib-ex/src` code the converters
  are checked against. The committed golden files were generated with RTKLIB
  Explorer commit `89a735ba8ff5038b2b556b267913a617d7210dd4`
  (`CONVBIN EX 2.5.0`); regenerate from the same commit unless intentionally
  updating the reference.
- `satpulsetool pack` extracts a raw packet byte stream from a SatPulse JSONL
  packet log, filtered by `--tag` and `--msg`. The packet logs themselves are
  not committed here; the raw streams below were produced from them.

## Fixtures

Each input file is paired with a `.obs.gz` golden of the same base name.

### `m8t-20251217-4h.ubx`, `f9t-20251217-3h.ubx`

Raw UBX-RXM-RAWX captures from u-blox M8T and F9T receivers.

    convbin -r ubx -v 3.04 -od -os -ro "-MULTICODE -MAX_STD_CP=15" \
      -o <name>.obs <name>.ubx

The test passes `--ubx-bds-geo-half-cycle` to match RTKLIB Explorer's BDS GEO
half-cycle phase correction.

### `packet-rtcm-20260519-3h.rtcm`

RTCM 3 MSM7 stream, extracted from a packet log:

    satpulsetool pack -t rtcm <packet-log>.jsonl > packet-rtcm-20260519-3h.rtcm

    convbin -r rtcm3 -v 3.04 -od -os -ro "-MULTIMODE -INVPRR" \
      -tr 2026/5/19 0:0:0 -o packet-rtcm-20260519-3h.obs \
      packet-rtcm-20260519-3h.rtcm

### `um980-rtcm-20260527-3h.rtcm`

RTCM 3 MSM7 stream from a Unicore UM980, extracted from the UM980 packet log:

    satpulsetool pack -t rtcm <packet-log>.jsonl > um980-rtcm-20260527-3h.rtcm

    convbin -r rtcm3 -v 3.04 -od -os -ro "-MULTIMODE -INVPRR" \
      -tr 2026/5/27 0:0:0 -o um980-rtcm-20260527-3h.obs \
      um980-rtcm-20260527-3h.rtcm

Unlike the 2026-05-19 stream, this one carries an RTCM 1005 station-ID
message.

### `um980-uncb-20260527-1h.uncb`

Unicore OBSVM stream from the same UM980, a one-hour slice. Only OBSVM
messages are needed for observation conversion, so the stream is filtered to
OBSVM with `--msg`:

    satpulsetool pack -t uncb -m OBSVM <packet-log>.jsonl \
      > um980-uncb-20260527-1h.uncb

    convbin -r unicore -v 3.04 -od -os \
      -o um980-uncb-20260527-1h.obs um980-uncb-20260527-1h.uncb

The test passes `--unc-omit-do-without-cp` because RTKLIB Explorer zeroes
Doppler when carrier-phase tracking is lost.
