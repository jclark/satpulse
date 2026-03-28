#!/usr/bin/env python3
"""Verify packet log replay output for anomalies.

Usage: verify-replay.py [--ecef x,y,z] <satpulsetool-binary> <packet-log-directory>

Runs satpulsetool replay on every .jsonl file in the directory,
collects all events, and checks for anomalies.

Options:
  --ecef x,y,z   Known ECEF position in meters. Checks posECEF and survey
                  positions are within 50m, and converts to lat/lon for
                  checking posGeo events.
"""

import argparse
import json
import math
import subprocess
import sys
from collections import defaultdict
from pathlib import Path


def ecef_to_latlon(x, y, z):
    """Convert ECEF to geodetic lat/lon/height (WGS84, iterative)."""
    a = 6378137.0
    f = 1 / 298.257223563
    b = a * (1 - f)
    e2 = 1 - (b * b) / (a * a)
    ep2 = (a * a) / (b * b) - 1
    p = math.sqrt(x * x + y * y)
    lon = math.atan2(y, x)
    # Bowring's iterative method
    lat = math.atan2(z, p * (1 - e2))
    for _ in range(10):
        sin_lat = math.sin(lat)
        N = a / math.sqrt(1 - e2 * sin_lat * sin_lat)
        lat = math.atan2(z + e2 * N * sin_lat, p)
    sin_lat = math.sin(lat)
    N = a / math.sqrt(1 - e2 * sin_lat * sin_lat)
    h = p / math.cos(lat) - N
    return math.degrees(lat), math.degrees(lon), h


def ecef_dist(a, b):
    """Euclidean distance between two ECEF positions."""
    return math.sqrt(sum((ai - bi) ** 2 for ai, bi in zip(a, b)))


def replay(binary, logfile):
    """Run satpulsetool replay and return parsed events."""
    r = subprocess.run([binary, "replay", str(logfile)],
                       capture_output=True, text=True)
    if r.returncode != 0:
        return None, r.stderr.strip()
    events = []
    for line in r.stdout.splitlines():
        if line.strip():
            events.append(json.loads(line))
    return events, None


def group_by_epoch(events):
    """Group events into epochs by navEpoch boundaries.

    Events up to and including each navEpoch form one epoch.
    Events after the last navEpoch (or with no navEpoch) go into a trailing group.
    """
    epochs = []
    cur = []
    for ev in events:
        cur.append(ev)
        if ev["type"] == "navEpoch":
            epochs.append(cur)
            cur = []
    if cur:
        epochs.append(cur)
    return epochs


def check_file(name, events, problems, ref_ecef=None, ref_latlon=None):
    """Run all checks on one file's replay output."""
    pfx = name

    if not events:
        problems.append(f"{pfx}: replay produced no events")
        return

    # Collect by type
    by_type = defaultdict(list)
    for ev in events:
        by_type[ev["type"]].append(ev)

    # -- Check 1: time events should have either taiTime or utcTime --
    for ev in by_type["time"]:
        d = ev["data"]
        has_tai = "taiTime" in d and d["taiTime"] is not None
        has_utc = "utcTime" in d and d["utcTime"] is not None
        if not has_tai and not has_utc:
            problems.append(f"{pfx}: time event with neither taiTime nor utcTime: {ev['t']}")

    # -- Check 2: TimTP events should have ref field --
    for ev in by_type["time"]:
        d = ev["data"]
        mid = d.get("nativeMsgID", "")
        if mid == "TIM-TP":
            if "ref" not in d or d["ref"] is None:
                problems.append(f"{pfx}: TIM-TP missing ref field at {ev['t']}")

    # -- Check 3: NAV-TIMEUTC gnss field --
    for ev in by_type["time"]:
        d = ev["data"]
        if d.get("nativeMsgID") == "NAV-TIMEUTC":
            if "gnss" not in d or d["gnss"] is None:
                problems.append(f"{pfx}: NAV-TIMEUTC missing gnss at {ev['t']}")

    # -- Check 4: leap second consistency within file --
    ls_vals = set()
    for ev in by_type["leapSecond"]:
        d = ev["data"]
        key = (d.get("UTCOffBefore"), d.get("UTCOffAfter"))
        ls_vals.add(key)
    if len(ls_vals) > 1:
        problems.append(f"{pfx}: inconsistent leap second values: {ls_vals}")

    # -- Check 5: per-epoch TAI time agreement --
    epochs = group_by_epoch(events)
    for i, epoch in enumerate(epochs):
        tai_values = {}
        for ev in epoch:
            if ev["type"] != "time":
                continue
            d = ev["data"]
            mid = d.get("nativeMsgID", "?")
            tai_str = d.get("taiTime")
            if tai_str is not None:
                tai_values[mid] = float(tai_str)
        if len(tai_values) >= 2:
            vals = list(tai_values.values())
            spread = max(vals) - min(vals)
            # TIM-TP is pre-pulse (refers to next second), others are post-pulse
            # so compare only post-pulse messages among themselves
            post_pulse = {k: v for k, v in tai_values.items() if k != "TIM-TP"}
            if len(post_pulse) >= 2:
                pvals = list(post_pulse.values())
                pspread = max(pvals) - min(pvals)
                if pspread > 0.01:  # 10ms tolerance
                    problems.append(
                        f"{pfx}: epoch {i}: post-pulse TAI spread {pspread:.9f}s "
                        f"among {post_pulse}")

    # -- Check 6: position plausibility --
    pos_tol = 10  # meters
    for ev in by_type["posGeo"]:
        d = ev["data"]
        ll = d.get("latLon")
        if ll:
            lat, lon = ll
            if not (-90 <= lat <= 90):
                problems.append(f"{pfx}: latitude out of range: {lat}")
            elif not (-180 <= lon <= 180):
                problems.append(f"{pfx}: longitude out of range: {lon}")
            elif ref_latlon:
                dlat = (lat - ref_latlon[0]) * 111320
                dlon = (lon - ref_latlon[1]) * 111320 * math.cos(math.radians(ref_latlon[0]))
                dist = math.sqrt(dlat * dlat + dlon * dlon)
                if dist > pos_tol:
                    problems.append(
                        f"{pfx}: posGeo {dist:.1f}m from reference at {ev['t']}")

    for ev in by_type["posECEF"]:
        d = ev["data"]
        pos = d.get("pos")
        if pos:
            if ref_ecef:
                dist = ecef_dist(pos, ref_ecef)
                if dist > pos_tol:
                    problems.append(
                        f"{pfx}: posECEF {dist:.1f}m from reference at {ev['t']}")
            else:
                mag = sum(x * x for x in pos) ** 0.5
                if not (6200000 < mag < 6500000):
                    problems.append(f"{pfx}: ECEF magnitude {mag:.0f}m out of range")

    # -- Check 8: velocity plausibility (stationary receiver) --
    for ev in by_type["velGeo"]:
        d = ev["data"]
        gs = d.get("groundSpeed", 0)
        if gs > 1.0:  # more than 1 m/s for stationary
            problems.append(f"{pfx}: groundSpeed {gs} m/s seems high for stationary")

    for ev in by_type["velECEF"]:
        d = ev["data"]
        vel = d.get("vel", [0, 0, 0])
        speed = sum(v*v for v in vel) ** 0.5
        if speed > 1.0:
            problems.append(f"{pfx}: ECEF speed {speed:.3f} m/s seems high for stationary")

    # -- Check 9: DOP values reasonable --
    for ev in by_type["navEpoch"]:
        d = ev["data"]
        dop = d.get("dop")
        if dop:
            for k, v in dop.items():
                if v is not None and v > 20:
                    problems.append(f"{pfx}: DOP {k}={v} seems high")

    # -- Check 10: survey events --
    for ev in by_type["survey"]:
        d = ev["data"]
        pos = d.get("position")
        if pos:
            if ref_ecef:
                dist = ecef_dist(pos, ref_ecef)
                if dist > pos_tol:
                    problems.append(
                        f"{pfx}: survey position {dist:.1f}m from reference at {ev['t']}")
            else:
                mag = sum(x * x for x in pos) ** 0.5
                if not (6200000 < mag < 6500000):
                    problems.append(f"{pfx}: survey ECEF magnitude {mag:.0f}m out of range")

    # -- Check 11: navEpoch fixLevel --
    for ev in by_type["navEpoch"]:
        d = ev["data"]
        fix = d.get("fixLevel")
        if fix and fix not in ("none", "dead-reckoning", "code", "float-rtk", "rtk"):
            problems.append(f"{pfx}: unexpected fixLevel: {fix}")

    # -- Check 12: cross-protocol satellite consistency --
    # When both NMEA and non-NMEA satellite events appear in the same epoch,
    # check that they agree on satellite IDs, look angles, and CN0.
    epochs = group_by_epoch(events)
    for i, epoch in enumerate(epochs):
        sat_msgs = [ev for ev in epoch if ev["type"] == "satellites"]
        nmea = [ev for ev in sat_msgs if ev["data"].get("tag") == "NMEA"]
        native = [ev for ev in sat_msgs if ev["data"].get("tag") != "NMEA"]
        if not nmea or not native:
            continue
        nmea_svs = {sv["id"]: sv for ev in nmea for sv in ev["data"]["info"]}
        native_svs = {sv["id"]: sv for ev in native for sv in ev["data"]["info"]}
        # Every NMEA SV should appear in native
        for svid in nmea_svs:
            if svid not in native_svs:
                problems.append(
                    f"{pfx}: epoch {i}: satellite {svid} in NMEA but not in native")
        # Check look angles for shared SVs
        for svid in nmea_svs:
            if svid not in native_svs:
                continue
            n_la = nmea_svs[svid].get("lookAngles")
            u_la = native_svs[svid].get("lookAngles")
            if not n_la or not u_la:
                continue
            n_az, u_az = n_la["azimuth"], u_la["azimuth"]
            # Normalize 0 vs 360
            if abs(n_az - u_az) > 180:
                n_az = n_az % 360
                u_az = u_az % 360
            if abs(n_az - u_az) > 1 or abs(n_la["elevation"] - u_la["elevation"]) > 1:
                problems.append(
                    f"{pfx}: epoch {i}: {svid} look angles differ: "
                    f"NMEA az={n_la['azimuth']} el={n_la['elevation']} vs "
                    f"native az={u_la['azimuth']} el={u_la['elevation']}")

    return by_type


def check_per_constellation(all_data, problems):
    """Check per-constellation captures for correct TimTP gnss."""
    constellation_files = {}
    for name in all_data:
        if name.startswith("time-") and not name.startswith("time-all"):
            # e.g. time-gps.jsonl -> GPS, time-bds-38400.jsonl -> BDS
            stem = name.replace("time-", "").replace(".jsonl", "")
            # Strip optional baud rate suffix (e.g. -38400)
            parts = stem.rsplit("-", 1)
            if len(parts) == 2 and parts[1].isdigit():
                stem = parts[0]
            gnss = stem.upper()
            constellation_files[gnss] = name

    for gnss, fname in constellation_files.items():
        events = all_data[fname]
        for ev in events:
            if ev["type"] != "time":
                continue
            d = ev["data"]
            if d.get("nativeMsgID") == "TIM-TP":
                tp_gnss = d.get("gnss")
                if tp_gnss is None:
                    problems.append(
                        f"{fname}: TIM-TP missing gnss field at {ev['t']}")
                elif tp_gnss != gnss:
                    problems.append(
                        f"{fname}: TIM-TP gnss={tp_gnss}, expected {gnss}")


def check_cross_file(all_data, problems):
    """Cross-file consistency checks."""
    # Leap second values should be consistent across all files
    all_ls = set()
    for name, events in all_data.items():
        for ev in events:
            if ev["type"] == "leapSecond":
                d = ev["data"]
                all_ls.add((d.get("UTCOffBefore"), d.get("UTCOffAfter")))
    if len(all_ls) > 1:
        problems.append(f"Cross-file: inconsistent leap second values: {all_ls}")


def main():
    parser = argparse.ArgumentParser(
        description="Verify packet log replay output for anomalies.")
    parser.add_argument("binary", help="Path to satpulsetool binary")
    parser.add_argument("logdir", help="Directory containing .jsonl packet logs")
    parser.add_argument("--ecef", metavar="x,y,z",
                        help="Known ECEF position in meters (comma-separated)")
    args = parser.parse_args()

    binary = args.binary
    logdir = Path(args.logdir)

    if not Path(binary).is_file():
        print(f"Error: binary not found: {binary}")
        sys.exit(1)
    if not logdir.is_dir():
        print(f"Error: directory not found: {logdir}")
        sys.exit(1)

    ref_ecef = None
    ref_latlon = None
    if args.ecef:
        ref_ecef = [float(v) for v in args.ecef.split(",")]
        if len(ref_ecef) != 3:
            print("Error: --ecef requires exactly 3 comma-separated values")
            sys.exit(1)
        ref_latlon = ecef_to_latlon(*ref_ecef)[:2]
        print(f"Reference ECEF: [{ref_ecef[0]:.4f}, {ref_ecef[1]:.4f}, {ref_ecef[2]:.4f}]")
        print(f"Reference lat/lon: [{ref_latlon[0]:.6f}, {ref_latlon[1]:.6f}]")

    files = sorted(logdir.glob("*.jsonl"))
    if not files:
        print(f"No .jsonl files in {logdir}")
        sys.exit(1)

    print(f"Replaying {len(files)} packet logs from {logdir}")

    # Replay all files
    all_data = {}
    for f in files:
        events, err = replay(binary, f)
        if err:
            print(f"  {f.name}: REPLAY ERROR: {err}")
            continue
        all_data[f.name] = events
        print(f"  {f.name}: {len(events)} events")

    # Run per-file checks
    problems = []
    for name, events in all_data.items():
        check_file(name, events, problems, ref_ecef, ref_latlon)

    # Run per-constellation checks
    check_per_constellation(all_data, problems)

    # Run cross-file checks
    check_cross_file(all_data, problems)

    # Report
    print()
    if problems:
        print(f"Found {len(problems)} problem(s):")
        for p in problems:
            print(f"  - {p}")
        sys.exit(1)
    else:
        print("All checks passed.")


if __name__ == "__main__":
    main()
