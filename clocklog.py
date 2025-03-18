# Python program to analyze SatPulse clock log files.
# Usage: python3 clocklog.py  [--all] <log_file>
# The --all flag can be used to analyze the entire log instead of only the last run.
import argparse
from dataclasses import dataclass

@dataclass
class LogEntry:
    date: str
    time: str
    offset: int
    freq: int
    outlier: bool
    era: int
    sync: bool

def parse_log(file_path, analyze_all=False):
    runs = []
    cur_run = []

    with open(file_path, "r") as file:
        for line in file:
            line = line.strip()

            if line.startswith("#"):
                if analyze_all and cur_run:
                    runs.append(cur_run)  # Only save completed runs if analyzing all
                cur_run = []  # Reset for a new run
                continue

            fields = line.split()
            if len(fields) != 7:
                continue  # Ignore malformed lines

            entry = LogEntry(
                fields[0],  # date
                fields[1],  # time
                int(fields[2]),  # offset
                int(fields[3]),  # freq
                bool(int(fields[4])),  # outlier (1 → True, 0 → False)
                int(fields[5]),  # era
                bool(int(fields[6]))   # sync (1 → True, 0 → False)
            )

            cur_run.append(entry)

    if cur_run:
        runs.append(cur_run)  # Always save the last run

    if not runs:
        print("No valid data found in log file.")
        return
    analyze_data(runs, analyze_all)

def analyze_data(runs, analyze_all):
    # Track sync statistics
    times_to_sync = []
    out_of_sync_ignore = 0
    sync_loss_count = 0
    
    for run in runs:
        synced = False
        
        for i, entry in enumerate(run):
            # Track first sync
            if not synced and entry.sync:
                synced = True
                times_to_sync.append(i + 1)  # +1 for 1-based counting
                # Ignore out-of-sync entries before first sync
                out_of_sync_ignore += i
                
            # Count sync losses (only after first sync)
            if synced and not entry.sync and run[i-1].sync:
                sync_loss_count += 1
        
        # If never synced, ignore the entire run as initial out-of-sync
        if not synced:
            out_of_sync_ignore += len(run)
    
    # Calculate total entries that were out of sync across all runs
    total_out_of_sync = sum(1 for run in runs for entry in run if not entry.sync)
    
    # Calculate out-of-sync time after initial sync
    lost_sync_duration = total_out_of_sync - out_of_sync_ignore
    
    # Analyze offset data
    data = [entry for run in runs for entry in run] if analyze_all else runs[-1]
    offsets = [abs(entry.offset) for entry in data if entry.sync and not entry.outlier]
    n_outliers = sum(1 for entry in data if entry.outlier)

    # Calculate statistics on valid offsets
    if offsets:
        max_offset = max(offsets)
        mean_offset = sum(offsets) / len(offsets)
        
        # Calculate 95th percentile
        offsets.sort()
        index = int(0.95 * len(offsets))
        percentile_95 = offsets[index]
    else:
        max_offset = 0
        mean_offset = 0
        percentile_95 = 0
    
    # Print summary statistics
    if analyze_all:
        print(f"Runs that synced: {len(times_to_sync)}/{len(runs)}")
        if times_to_sync:
            mean_time_to_sync = sum(times_to_sync) / len(times_to_sync)
            print(f"Mean time to sync: {mean_time_to_sync:.2f}s")
            print(f"Max time to sync: {max(times_to_sync)}s")
    else:
        # For single run analysis
        print(f"Synced: {len(times_to_sync) > 0}")
        if times_to_sync:
            print(f"Time to sync: {times_to_sync[0]}s")
        
    print(f"Number of sync losses: {sync_loss_count}")
    print(f"Total time where sync lost: {lost_sync_duration}s")
    
    print(f"Max offset: {max_offset}ns")
    print(f"Mean offset: {mean_offset:.2f}ns")
    print(f"95% offset: {percentile_95}ns")
    print(f"Number of outliers: {n_outliers}")

if __name__ == "__main__":
    parser = argparse.ArgumentParser(description="Analyze SatPulse clock log.")
    parser.add_argument("file", help="Path to the SatPulse clock log file")
    parser.add_argument("--all", action="store_true", help="Analyze the entire log instead of only the last run")
    
    args = parser.parse_args()
    parse_log(args.file, args.all)