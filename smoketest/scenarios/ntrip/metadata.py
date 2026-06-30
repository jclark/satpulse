"""Ntrip caster source-table metadata defaults and overrides.

The shared [ntrip] STR fields (network, country, generator, lat, lon,
bitrate) appear on every mountpoint's STR record. The FULL mountpoint
overrides the identifier (description) and bitrate; the BASE mountpoint
takes the per-mount defaults (identifier = name, shared bitrate).
"""

import common
from scenarios import ntrip

PACKET_LOG = "gps/testdata/packets/unicore/UM982/rtcm-eph.jsonl"
FACTOR = 10


def _check_shared(rec: list[str]) -> None:
    assert rec[ntrip.STR_NETWORK] == "SmokeNet", rec
    assert rec[ntrip.STR_COUNTRY] == "THA", rec
    assert rec[ntrip.STR_GENERATOR] == "smoke-generator", rec
    assert rec[ntrip.STR_LAT] == "13.76", rec
    assert rec[ntrip.STR_LON] == "100.50", rec


def run(ctx: common.SmokeContext) -> None:
    full = ntrip.sourcetable_record(ctx, "FULL")
    _check_shared(full)
    assert full[ntrip.STR_IDENTIFIER] == "Full metadata mount", full
    assert full[ntrip.STR_BITRATE] == "9600", full

    base = ntrip.sourcetable_record(ctx, "BASE")
    _check_shared(base)
    assert base[ntrip.STR_IDENTIFIER] == "BASE", base
    assert base[ntrip.STR_BITRATE] == "4800", base

    ctx.wait_replay()
