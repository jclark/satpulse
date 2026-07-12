"""satpulsewb x u-blox simulator: the interactive config path, UI-shaped.

Starts the workbench without -d and drives the config path the way the browser
does: claim the write seat, connect to the simulator's pty (the first black-box
exercise of an interactive connect whose probe answers), wait for /api/receiver
to identify the simulated ZED-F9P, read the config, apply a small ConfigTarget
that both sets a round-trippable property (the antenna cable delay) and enables
the satellites messages (Opts.SatsMsg), then confirm a second read shows the new
delay and the enabled messages arrive as live satellite data.

No FACTOR/PACKET_LOG: the simulator generates nav itself from SIM_REPLAY.
"""

import json

import common
from scenarios import config

PROGRAM = "satpulsewb"
PROVIDER = "ubxsim"
PERSONALITY = "gps/app/ubxsim/testdata/f9p/f9p-personality.ubx"
SIM_REPLAY = "gps/testdata/config/u-blox/ZED-F9P/sim.jsonl"

# A round-trippable property the simulator's config database stores and returns
# verbatim. Nanoseconds on the wire (ConfigProps marshals antennaCableDelay as
# an integer nanosecond count).
CABLE_DELAY_NS = 12000
# Opts.SatsMsg wire value: satellite positions (NAV-SAT) plus signals (NAV-SIG),
# the same flag names the config panel sends (SatsMsgFlags marshals as an array
# of names).
SATS_MSG = ["sat", "signal"]


def run(ctx: common.SmokeContext) -> None:
    seat = common.wb_claim(ctx)  # connect and config are writer POSTs

    status, body = common.wb_post(
        ctx, f"/api/connect?seat={seat}", {"device": ctx.serial, "speed": 38400}
    )
    assert status == 200, f"connect failed: {status} {body!r}"

    config.check_wb_receiver_identity(ctx)

    status, body = common.wb_post(ctx, f"/api/config/read?seat={seat}", {})
    assert status == 200, f"config/read expected 200, got {status}: {body!r}"
    props = json.loads(body)
    assert props, f"config/read returned empty props: {body!r}"

    status, body = common.wb_post(
        ctx,
        f"/api/config/apply?seat={seat}",
        {"Props": {"antennaCableDelay": CABLE_DELAY_NS}, "Opts": {"SatsMsg": SATS_MSG}},
    )
    assert status == 200, f"config/apply expected 200, got {status}: {body!r}"

    # The property round-trips: a second read reflects the applied delay.
    status, body = common.wb_post(ctx, f"/api/config/read?seat={seat}", {})
    assert status == 200, f"config/read (2) expected 200, got {status}: {body!r}"
    props2 = json.loads(body)
    assert props2.get("antennaCableDelay") == CABLE_DELAY_NS, (
        f"antennaCableDelay did not round-trip: sent {CABLE_DELAY_NS}, read {props2.get('antennaCableDelay')!r}"
    )

    # The enabled messages show up as live data.
    config.check_wb_sats_live(ctx)
