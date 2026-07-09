#!/bin/bash
# Capture the u-blox X20P personality artifacts for the receiver
# simulator (#362). Run this on the machine with the X20P attached.
#
# Produces in OUTDIR:
#   x20p-personality.ubx    the personality file (MON-VER, MON-GNSS and
#                           the Default-layer CFG-VALGET dump), the
#                           required argument of satpulsetool ubxsim
#   x20p-personality.jsonl  packet log of the personality capture
#   x20p-enable.jsonl       the message-enable exchange (for reference)
#   x20p-replay.jsonl       the replay recording for the NAV engine
#
# The output-protocol and message enables and the rate change are
# written to the RAM layer only; a power cycle restores the receiver's
# saved configuration. The enables set everything the recording needs
# (output protocols, 1 Hz rate, all messages), so the recording does
# not depend on the receiver's current configuration -- but RTCM
# TYPE1005 (ARP) and meaningful NAV-SVIN content appear only if the
# receiver is in base mode (TMODE fixed or survey-in).

set -euo pipefail

dev=/dev/ttyACM0
speed=38400
port=usb
dur=900
outdir=x20p-artifacts

usage() {
    echo "usage: $0 [-d device] [-s speed] [-p port] [-t seconds] [-o outdir]" >&2
    echo "  -d  serial device (default $dev)" >&2
    echo "  -s  baud rate (default $speed; nominal on USB)" >&2
    echo "  -p  receiver port for message enables (default $port)" >&2
    echo "  -t  replay recording duration in seconds (default $dur)" >&2
    echo "  -o  output directory (default $outdir)" >&2
    echo "env: SATPULSETOOL overrides the satpulsetool binary" >&2
    exit 2
}

while getopts d:s:p:t:o: opt; do
    case $opt in
    d) dev=$OPTARG ;;
    s) speed=$OPTARG ;;
    p) port=$OPTARG ;;
    t) dur=$OPTARG ;;
    o) outdir=$OPTARG ;;
    *) usage ;;
    esac
done
shift $((OPTIND - 1))
[ $# -eq 0 ] || usage

scriptdir=$(cd "$(dirname "$0")" && pwd)
toml=$scriptdir/x20p-capture.toml
[ -f "$toml" ] || { echo "$0: $toml not found" >&2; exit 1; }

if [ -z "${SATPULSETOOL:-}" ]; then
    case $(uname -m) in
    x86_64) arch=amd64 ;;
    aarch64) arch=arm64 ;;
    *) arch=$(uname -m) ;;
    esac
    SATPULSETOOL=$scriptdir/../../../../out/$arch/satpulsetool
    if [ ! -x "$SATPULSETOOL" ]; then
        SATPULSETOOL=$(command -v satpulsetool) ||
            { echo "$0: satpulsetool not found; build with make or set SATPULSETOOL" >&2; exit 1; }
    fi
fi

if pgrep -x satpulsed >/dev/null; then
    echo "$0: satpulsed is running; stop it first (it owns the receiver)" >&2
    exit 1
fi

mkdir -p "$outdir"
enable_tmp=$outdir/.x20p-enable
for f in x20p-personality.ubx x20p-personality.jsonl x20p-enable.jsonl x20p-replay.jsonl; do
    if [ -e "$outdir/$f" ]; then
        echo "$0: $outdir/$f already exists; rename or move it first (packet logs are append-only, never delete captures)" >&2
        exit 1
    fi
done
if [ -e "$enable_tmp" ]; then
    echo "$0: $enable_tmp already exists; rename or move it first (staged enable packet logs from an interrupted run)" >&2
    exit 1
fi

echo "== 1/3: personality (MON-VER, MON-GNSS, Default-layer dump) -> $outdir/x20p-personality.ubx"
echo "   (CFG-VALGET polls past the end of the database return empty pages on the X20; F9P-era firmware NAKs them. Both are expected.)"
"$SATPULSETOOL" gps -d "$dev" -s "$speed" -m "$toml" -t mon-ver,mon-gnss,valget-default \
    --packet-log "$outdir/x20p-personality.jsonl"
"$SATPULSETOOL" pack -t UBX "$outdir/x20p-personality.jsonl" > "$outdir/x20p-personality.ubx"

echo "== 2/3: enable output protocols and messages (RAM only, port $port) -> $outdir/x20p-enable.jsonl"
mkdir "$enable_tmp"
"$SATPULSETOOL" gps -d "$dev" -s "$speed" -m "$toml" -t rate-1hz \
    --packet-log "$enable_tmp/rate-1hz.jsonl"
"$SATPULSETOOL" gps -d "$dev" -s "$speed" -m "$toml" -t enable-out,enable-msgs,enable-rtcm --port "$port" \
    --packet-log "$enable_tmp/enable-msgs.jsonl"
"$SATPULSETOOL" gps -d "$dev" -s "$speed" -m "$toml" -t tmode-svin \
    --packet-log "$enable_tmp/tmode-svin.jsonl"
cat "$enable_tmp/rate-1hz.jsonl" "$enable_tmp/enable-msgs.jsonl" "$enable_tmp/tmode-svin.jsonl" > "$enable_tmp/x20p-enable.jsonl"
mv "$enable_tmp/x20p-enable.jsonl" "$outdir/x20p-enable.jsonl"
rm -r "$enable_tmp"

echo "== 3/3: replay recording, $dur seconds -> $outdir/x20p-replay.jsonl"
"$SATPULSETOOL" gps -d "$dev" -s "$speed" --capture "$dur" \
    --packet-log "$outdir/x20p-replay.jsonl"

echo "== done"
ls -l "$outdir"
echo "Copy x20p-personality.ubx and x20p-replay.jsonl to gps/app/ubxsim/testdata/x20p/ in the ublox-sim worktree."
