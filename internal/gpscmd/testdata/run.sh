#!/bin/bash
# Run test cases for GPS configuration
# Usage: ./run.sh <receiver-config> <commands-file>
# Example: ./run.sh f9p.sh pvt-out.sh

set -e

if [ $# -ne 2 ]; then
    echo "Usage: $0 <receiver-config> <commands-file>"
    echo "Example: $0 f9p.sh cmd/pvt-out.sh"
    exit 1
fi

receiver_config=$1
commands_file=$2

# Extract base names without .sh extension
receiver_base=$(basename "$receiver_config" .sh)
# Handle path in commands_file (e.g., cmd/pvt-out.sh -> pvt-out)
commands_base=$(basename "$commands_file" .sh)

# Source receiver configuration
source "$receiver_config"

# Determine architecture
arch=$(uname -m)
if [ "$arch" = "x86_64" ]; then
    arch="amd64"
elif [ "$arch" = "aarch64" ]; then
    arch="arm64"
fi

# Detect OS and determine output directory
os=$(uname -s | tr '[:upper:]' '[:lower:]')
if [ "$os" = "linux" ]; then
    outdir="$arch"
else
    outdir="${os}_${arch}"
fi

PATH="../../../out/$outdir:$PATH"
export PATH

# Output file name
if [ -n "$vendor" ]; then
    mkdir -p "$vendor"
    output_file="${vendor}/${receiver_base}-${commands_base}.jsonl"
    anno_output_file="${vendor}/${receiver_base}-${commands_base}.anno.jsonl"
else
    output_file="${receiver_base}-${commands_base}.jsonl"
    anno_output_file="${receiver_base}-${commands_base}.anno.jsonl"
fi

# Function to create numbered backups (GNU Emacs style)
backup_file() {
    local file="$1"
    if [ -f "$file" ]; then
        # Find the highest numbered backup using a pipeline
        local max_num=$(ls "${file}"~*~ 2>/dev/null | \
                        sed "s|^${file}~\([0-9]*\)~$|\1|" | \
                        sort -n | \
                        tail -1)
        # Default to 0 if no backups exist or parsing failed
        max_num=${max_num:-0}
        # Create next numbered backup
        local next_num=$((max_num + 1))
        local backup_name="${file}~${next_num}~"
        mv "$file" "$backup_name"
        echo "Backed up existing $file to $backup_name"
    fi
}

# Backup existing output files
backup_file "$output_file"
backup_file "$anno_output_file"

# Build common flags
vendor_flag=""
if [ -n "$vendor" ]; then
    vendor_flag="--vendor $vendor"
fi

# Define the t function that runs satpulsetool
# Use a temp file and append, since --test-log truncates
t() {
    local tmpfile=$(mktemp)
    echo Running: satpulsetool gps -d /dev/$dev -s $speed $vendor_flag --test-log $output_file "$@"
    satpulsetool gps -d "/dev/$dev" -s "$speed" $vendor_flag --test-log "$tmpfile" "$@"
    sed "s|$tmpfile|$output_file|" "$tmpfile" >> "$output_file"
    rm "$tmpfile"
}

# Display version first
satpulsetool --version

# Discard temp state
echo Running: satpulsetool gps --reload
satpulsetool gps --reload -d /dev/$dev -s $speed $vendor_flag
sleep ${reload_secs:-1}

echo Setting binary mode
satpulsetool gps --binary -d /dev/$dev -s $speed $vendor_flag

# Switch to high speed for tests that need more bandwidth
use_high_speed=false
if [ -n "$high_speed_tests" ] && [ -n "$high_speed" ] && [ -n "$high_speed_msgfile" ]; then
    for ht in $high_speed_tests; do
        if [ "$commands_base" = "$ht" ]; then
            echo "Switching to high speed ($high_speed) for $commands_base"
            satpulsetool gps -d "/dev/$dev" -s "$speed" $vendor_flag -m "$high_speed_msgfile"
            speed=$high_speed
            use_high_speed=true
            break
        fi
    done
fi

set +e
# Source and run the commands file
source "$commands_file"

# Restore default speed if changed
if $use_high_speed && [ -n "$default_speed_msgfile" ]; then
    echo "Restoring default speed"
    satpulsetool gps -d "/dev/$dev" -s "$speed" $vendor_flag -m "$default_speed_msgfile"
fi

echo Test log written to: $output_file
satpulsetool annotate $output_file >$anno_output_file
