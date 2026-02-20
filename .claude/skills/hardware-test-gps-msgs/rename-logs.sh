#!/bin/bash
# Rename JSONL log files with a suffix to keep runs separate.
# Usage: rename-logs.sh <suffix>
# Example: rename-logs.sh f9p-ubx-baseline
set -eu
suffix="$1"
dir="tmp/satpulsed/log"
for f in "$dir"/*.jsonl; do
  [ -f "$f" ] && mv "$f" "${f%.jsonl}-${suffix}.jsonl"
done
