#!/bin/bash
# Regenerate test data files using Python simulator with Go RNG for reproducibility.
# Run from any directory - script finds its own location.

set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
SATPULSE="$(cd "$SCRIPT_DIR/../../../../.." && pwd)"
CLOCK_MODEL="$SATPULSE/../clock-model"

if [[ ! -d "$CLOCK_MODEL" ]]; then
    echo "Error: clock-model not found at $CLOCK_MODEL" >&2
    exit 1
fi

cd "$CLOCK_MODEL"

# PHC test data
for name in whiteNoise flickerNoise randomWalk sinusoid; do
    echo "Generating phc.$name.jsonl..."
    uv run simulate.py "$SCRIPT_DIR/phc.$name.toml" 600 --norm-rng-path ./gonormrng > "$SCRIPT_DIR/phc.$name.jsonl"
done

# GPS test data
for name in jitter ar1 ar1fm randomWalk drift sawtooth sinusoid resonator; do
    toml="$SCRIPT_DIR/gps.$name.toml"
    if [[ -f "$toml" ]]; then
        echo "Generating gps.$name.jsonl..."
        uv run simulate.py "$toml" 600 --norm-rng-path ./gonormrng > "$SCRIPT_DIR/gps.$name.jsonl"
    fi
done

echo "Done."