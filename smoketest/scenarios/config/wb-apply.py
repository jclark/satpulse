"""satpulsewb x u-blox simulator: the interactive config path, UI-shaped.

Starts the workbench without -d and drives the config path the way the browser
does: claim the write seat, connect to the simulator's pty (the first black-box
exercise of an interactive connect whose probe answers), wait for /api/receiver
to identify the simulated ZED-F9P, read the config, then apply ConfigTargets
carrying a round-trippable property (the antenna cable delay) and every message
flag group Opts has, and confirm they land -- a re-read shows the new delay, and
the receiver sends exactly the messages that were enabled.

Every group crosses the wire as an array of names, so a regression in any group's
encoding fails the apply here. Four are checked as behaviour too, by naming the
packet each one enables on the workbench packet stream: PVTMsg (NAV-TIMELS),
SatsMsg (NAV-SAT/NAV-SIG) and RTCMMsg (MSM4, which the simulator derives from the
recorded MSM7) are off in the personality defaults, so their packets arriving can
only be the VALSET landing. NMEA defaults on, so it is cycled the other way --
disabled with the empty array, shown gone, then re-enabled by name -- which pins
both encodings. Only RawMsg is wire-format-only: the bank holds no RXM packet for
its keys to gate.

The second apply turns the satellite messages off as it turns RTCM on, keeping the
enabled set inside what the 38400-baud port can drain: the simulator paces its
output at the port's baud rate, so an over-subscribed link would just back up.

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
# The Opts flag groups as the config panel sends them, each an array of names,
# with the packet each one enables on this receiver.
PVT_MSG = ["leapSecond"]  # NAV-TIMELS
SATS_MSG = ["sat", "signal"]  # NAV-SAT, NAV-SIG
NMEA_MSG = ["GGA", "RMC"]  # GNGGA, GNRMC
RTCM_MSG = ["MSM4"]  # 1074 and its per-constellation siblings
RAW_MSG = ["obs", "navData"]  # RXM-RAWX/RXM-SFRBX: not in the bank, so not observable


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

    # Every flag group in one apply: the UBX messages on, NMEA off (it defaults on).
    apply(
        ctx,
        seat,
        {
            "Props": {"antennaCableDelay": CABLE_DELAY_NS},
            "Opts": {
                "PVTMsg": PVT_MSG,
                "SatsMsg": SATS_MSG,
                "NMEAMsg": [],
                "RTCMMsg": [],
                "RawMsg": RAW_MSG,
            },
        },
    )

    # The property round-trips: a second read reflects the applied delay.
    status, body = common.wb_post(ctx, f"/api/config/read?seat={seat}", {})
    assert status == 200, f"config/read (2) expected 200, got {status}: {body!r}"
    props2 = json.loads(body)
    assert props2.get("antennaCableDelay") == CABLE_DELAY_NS, (
        f"antennaCableDelay did not round-trip: sent {CABLE_DELAY_NS}, read {props2.get('antennaCableDelay')!r}"
    )

    config.check_wb_packets_absent(ctx, ["NMEA"])
    config.check_wb_packets_live(ctx, ["NAV-SAT", "NAV-SIG", "NAV-TIMELS"])

    # NMEA by name brings it back, and RTCM turns on what the defaults leave off.
    apply(ctx, seat, {"Opts": {"SatsMsg": [], "NMEAMsg": NMEA_MSG, "RTCMMsg": RTCM_MSG}})
    config.check_wb_packets_live(ctx, ["GNGGA", "GNRMC", "1074"])


def apply(ctx: common.SmokeContext, seat: str, target: dict[str, object]) -> None:
    status, body = common.wb_post(ctx, f"/api/config/apply?seat={seat}", target)
    assert status == 200, f"config/apply {target} expected 200, got {status}: {body!r}"
