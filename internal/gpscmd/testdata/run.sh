#!/bin/bash
# Run test cases for GPS configuration
# Usage: ./run.sh <receiver-config> <commands-file>
# Example: ./run.sh f9p.sh pvt-out.sh

set -e

if [ $# -ne 2 ]; then
    echo "Usage: $0 <receiver-config> <commands-file>"
    echo "Example: $0 f9p.sh pvt-out.sh"
    exit 1
fi

receiver_config=$1
commands_file=$2

# Extract base names without .sh extension
receiver_base=$(basename "$receiver_config" .sh)
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

PATH="../../../out/$arch:$PATH"
export PATH

# Output file name
output_file="${receiver_base}-${commands_base}.jsonl"
anno_output_file="${receiver_base}-${commands_base}.anno.jsonl"

# Function to create numbered backups (GNU Emacs style)
backup_file() {
    local file="$1"
    if [ -f "$file" ]; then
        # Find the highest numbered backup using a pipeline
        local max_num=$(ls "${file}"~*~ 2>/dev/null | \
                        sed "s/^${file}~\([0-9]*\)~$/\1/" | \
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

# Define the t function that runs satpulsetool
t() {
    echo Running: satpulsetool gps -d /dev/tty$tty -s $speed --test-log $output_file "$@"
    satpulsetool gps -d "/dev/tty$tty" -s "$speed" --test-log "$output_file" "$@"
}

# Display version first
satpulsetool --version

# Discard temp state
echo Running: satpulsetool gps --reload
satpulsetool gps --reload -d /dev/tty$tty -s $speed
sleep 1

echo Setting binary mode
satpulsetool gps --binary -d /dev/tty$tty -s $speed

# Source and run the commands file
source "$commands_file"

echo Test log written to: $output_file
ubxanno <$output_file >$anno_output_file
