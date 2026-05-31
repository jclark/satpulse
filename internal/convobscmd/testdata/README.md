# convobs golden test fixtures

These files back `TestGoldenFiles` in `convobs_test.go`. Each test case
converts a raw observation input with `convobs` and compares the result,
semantically, against a golden RINEX observation file produced by RTKLIB
Explorer's `convbin`. The comparison ignores metadata that depends on run
time, and a small whitelist of signals where SatPulse and `convbin` disagree
in ways the protocol specs resolve in SatPulse's favour (documented in the
test).

The golden files are therefore an independent reference (RTKLIB Explorer),
not SatPulse self-output. Regenerate them with `make` (see `Makefile`, which
holds the convbin flags for every case) only when the reference itself
changes, and review any resulting observation differences before committing.

## Tools

- `convbin` is the RTKLIB Explorer build, not the system `convbin`. Build it
  from `~/rtklib-ex` so it matches the `~/rtklib-ex/src` code the converters
  are checked against, and point `make` at it with
  `make CONVBIN=~/rtklib-ex/bin/convbin`. The committed golden files were
  generated with RTKLIB Explorer commit
  `89a735ba8ff5038b2b556b267913a617d7210dd4` (`CONVBIN EX 2.5.0`); regenerate
  from the same commit unless intentionally updating the reference.
- `satpulsetool pack` extracts a raw packet byte stream from a SatPulse JSONL
  packet log, filtered by `--tag` and `--msg`. The packet logs themselves are
  not committed here; the raw streams below were produced from them.

## Fixtures

Each input file is paired with a `.obs.gz` golden of the same base name. Every
raw stream is a 15-minute slice, long enough to cover all signals and exercise
the carrier-phase arc logic. The `Makefile` regenerates every golden; this
section describes only what each raw stream is and how it was extracted.

### `m8t-20251217.ubx`, `f9t-20251217.ubx`

Raw UBX-RXM-RAWX captures from u-blox M8T and F9T receivers.

### `packet-rtcm-20260519.rtcm`

RTCM 3 MSM7 stream, extracted from a packet log:

    satpulsetool pack -t rtcm <packet-log>.jsonl > packet-rtcm-20260519.rtcm

### `um980-rtcm-20260527.rtcm`

RTCM 3 MSM7 stream from a Unicore UM980, extracted from the UM980 packet log:

    satpulsetool pack -t rtcm <packet-log>.jsonl > um980-rtcm-20260527.rtcm

Unlike the 2026-05-19 stream, this one carries an RTCM 1005 station-ID
message.

### `um980-uncb-20260527.uncb`

Unicore OBSVM stream from the same UM980. Only OBSVM messages are needed for
observation conversion, so the stream is filtered to OBSVM with `--msg`:

    satpulsetool pack -t uncb -m OBSVM <packet-log>.jsonl \
      > um980-uncb-20260527.uncb
