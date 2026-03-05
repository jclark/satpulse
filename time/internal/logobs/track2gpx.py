#!/usr/bin/env python3
"""Convert a SatPulse track JSONL file to GPX 1.1 format.

Reads from a file argument or stdin, writes GPX XML to stdout.
"""

import argparse
import json
import sys
from datetime import datetime, timezone
from decimal import Decimal
from xml.sax.saxutils import quoteattr

GPX_HEADER = """\
<?xml version="1.0" encoding="UTF-8"?>
<gpx version="1.1" creator="track2gpx"
     xmlns="http://www.topografix.com/GPX/1/1"
     xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"
     xsi:schemaLocation="http://www.topografix.com/GPX/1/1 http://www.topografix.com/GPX/1/1/gpx.xsd">
<trk>
"""

GPX_FOOTER = """\
</trk>
</gpx>
"""


def parse_time(s):
    return datetime.fromisoformat(s.replace("Z", "+00:00"))


def write_trkpt(out, pt):
    out.write(f'   <trkpt lat={quoteattr(str(pt["lat"]))} lon={quoteattr(str(pt["lon"]))}>\n')
    # GPX 1.1 schema requires elements in this order
    if "ele" in pt:
        out.write(f"    <ele>{pt['ele']}</ele>\n")
    if "t" in pt:
        out.write(f"    <time>{pt['t']}</time>\n")
    if "fixType" in pt:
        out.write(f"    <fix>{pt['fixType']}</fix>\n")
    if "numsat" in pt:
        out.write(f"    <sat>{pt['numsat']}</sat>\n")
    if "hdop" in pt:
        out.write(f"    <hdop>{pt['hdop']}</hdop>\n")
    if "vdop" in pt:
        out.write(f"    <vdop>{pt['vdop']}</vdop>\n")
    if "pdop" in pt:
        out.write(f"    <pdop>{pt['pdop']}</pdop>\n")
    out.write("   </trkpt>\n")


def convert(inp, out, max_gap):
    out.write(GPX_HEADER)
    in_seg = False
    prev_time = None
    for line in inp:
        line = line.strip()
        if not line:
            continue
        pt = json.loads(line, parse_float=Decimal)
        if "lat" not in pt or "lon" not in pt:
            continue
        t = parse_time(pt["t"]) if "t" in pt else None
        if t is not None and prev_time is not None:
            gap = (t - prev_time).total_seconds()
            if gap > max_gap:
                out.write("  </trkseg>\n")
                in_seg = False
        if not in_seg:
            out.write("  <trkseg>\n")
            in_seg = True
        write_trkpt(out, pt)
        prev_time = t
    if in_seg:
        out.write("  </trkseg>\n")
    out.write(GPX_FOOTER)


def main():
    p = argparse.ArgumentParser(description="Convert SatPulse track JSONL to GPX 1.1")
    p.add_argument("file", nargs="?", help="track JSONL file (default: stdin)")
    p.add_argument("--max-gap", type=float, default=10,
                   help="max seconds between points before starting new segment (default: 10)")
    args = p.parse_args()
    if args.file:
        with open(args.file) as f:
            convert(f, sys.stdout, args.max_gap)
    else:
        convert(sys.stdin, sys.stdout, args.max_gap)


if __name__ == "__main__":
    main()
