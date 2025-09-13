#!/usr/bin/env python3
"""
SDP Loopback Test Script for Intel igb/igc Drivers

Tests the satpulsetool sdp command by generating periodic output on one pin
and verifying reception on another pin. Checks that pulses align to second
boundaries as expected for PPS-related periods.

NOTE: This test is specific to Intel igb/igc drivers which:
  - Generate 50% duty cycle pulses
  - Timestamp both rising and falling edges
  - Don't support the --width parameter

Usage:
    python3 sdp-intel-loopback-test.py <satpulsetool_path> <output_interface> <output_pin> <input_interface> <input_pin>

Example (in the satpulse source directory):
    sudo python3 systest/sdp-intel-loopback-test.py ./out/amd64/satpulsetool enp4s0 0 enp5s0 0

This assumes you have connected the output pin to the input pin with a cable.
"""

import sys
import subprocess
import json
import os
import re
from datetime import datetime
from decimal import Decimal
from typing import List, Optional, TextIO, Tuple, Dict, Any, TypedDict, Literal, cast

# Test periods and their expected edge alignments
# For 50% duty cycle signals, we get edges at every period/2 interval
# 1.0s period is first as a connectivity test - if it fails, we stop
TEST_PERIODS: List[Tuple[float, List[float]]] = [
    (1.0, [0.0, 0.5]),  # 1.0s period: edges at .000 and .500
    (2.0, [0.0]),  # 2.0s period: edge every 2 seconds at .000 (only one edge per second)
    (0.5, [0.0, 0.25, 0.5, 0.75]),  # 0.5s period: edges every 0.25s
    (0.2, [0.0, 0.1, 0.2, 0.3, 0.4, 0.5, 0.6, 0.7, 0.8, 0.9]),  # 0.2s period: edges every 0.1s
    (0.1, [0.0, 0.05, 0.1, 0.15, 0.2, 0.25, 0.3, 0.35, 0.4, 0.45, 0.5, 0.55, 0.6, 0.65, 0.7, 0.75, 0.8, 0.85, 0.9, 0.95]),  # 0.1s period: edges every 0.05s
]

ALIGNMENT_TOLERANCE: float = 0.002  # 2ms tolerance for alignment checks

# Global variables
satpulsetool: str = ""
LOG_FILE: Optional[TextIO] = None


def log(msg: str, to_stderr: bool = False) -> None:
    """Log to file and optionally to stderr."""
    if LOG_FILE:
        LOG_FILE.write(f"{datetime.now().strftime('%H:%M:%S.%f')[:-3]} {msg}\n")
        LOG_FILE.flush()
    if to_stderr:
        print(msg, file=sys.stderr)

def fatal(msg: Optional[str], code: int = 1) -> None:
    """Log and print an error, close log, and exit with code."""
    if msg:
        log(msg, to_stderr=True)
    if LOG_FILE:
        try:
            LOG_FILE.close()
        except Exception:
            pass
    sys.exit(code)


def run_command(cmd: List[str], quiet: bool = False) -> subprocess.CompletedProcess:
    """Run a command and return the result."""
    log(f"CMD: {' '.join(cmd)}")
    
    # Always capture output
    result = subprocess.run(cmd, capture_output=True, text=True)
    
    # Log stdout if not quiet
    if result.stdout and not quiet:
        for line in result.stdout.strip().split('\n'):
            if line:
                log(f"  OUT: {line}")
    
    # Always log stderr (contains verbose output from -v flag)
    if result.stderr:
        for line in result.stderr.strip().split('\n'):
            if line:
                log(f"  ERR: {line}")
    
    return result


def run_sdp_command(sdp_args: List[str], quiet: bool = False) -> Tuple[int, List[Dict[str, Any]]]:
    """Run a satpulsetool sdp command with verbose flag and parse JSON output.
    
    Args:
        sdp_args: Arguments to pass after 'sdp' (e.g., ['-o', '--pin', '0', 'eth0'])
        quiet: If True, don't log stdout
        
    Returns:
        Tuple of (exit_code, list of parsed JSON objects)
    """
    if "-j" not in sdp_args:
        sdp_args = ["-j"] + sdp_args
    
    cmd = [satpulsetool, "-v", "sdp"] + sdp_args
    result = run_command(cmd, quiet=quiet)
    
    # Parse JSON lines from stdout
    parsed_lines = []
    for line in result.stdout.strip().split('\n'):
        if line:
            try:
                parsed_lines.append(json.loads(line))
            except json.JSONDecodeError as e:
                log(f"Failed to parse JSON line: {e}")
                log(f"  Line was: {repr(line)}")
    
    return result.returncode, parsed_lines


class InterfaceInfo(TypedDict, total=False):
    name: str
    status: str
    driver: str
    pins: List[str]
    clock_index: int

PinFunction = Literal["none", "extts", "perout"]


def get_interface_info(interface: str) -> Optional[InterfaceInfo]:
    """Get interface info from satpulsetool sdp output."""
    # Get all interfaces and search for the one we want
    exit_code, interfaces = run_sdp_command([], quiet=False)
    if exit_code != 0:
        log(f"Interface discovery failed with exit code {exit_code}")
        return None
    
    for iface in cast(List[InterfaceInfo], interfaces):
        if iface.get("name") == interface:
            return iface
    return None


def get_phc_index(interface: str) -> Optional[int]:
    """Get PHC index for an interface."""
    info = get_interface_info(interface)
    return info.get("clock_index") if info else None



def sync_phc_clocks(iface1: str, iface2: str) -> None:
    """Sync PHC clocks if they are different interfaces."""
    if iface1 == iface2:
        log("Same interface for input and output, no PHC sync needed")
        return
    
    # Get current time from output interface PHC
    result = run_command(["phc_ctl", "-q", iface2, "get"])
    if result.returncode != 0:
        log("Warning: Could not get time from output PHC")
        return
    
    # Parse output to extract seconds.nanoseconds
    match = re.search(r"clock time is (\d+\.\d+)", result.stdout)
    if not match:
        log("Warning: Could not parse time from output PHC")
        return
    
    time_str = match.group(1)
    
    # Set input PHC to match output PHC
    log(f"Syncing {iface1} PHC to {iface2} PHC (time={time_str})")
    result = run_command(["phc_ctl", "-q", iface1, "set", time_str])
    if result.returncode != 0:
        log("Warning: PHC sync failed")


def test_period(input_iface: str, input_pin: str, 
               output_iface: str, output_pin: str,
               period: float, 
               expected_alignments: List[float]) -> Tuple[bool, float, int]:
    """Test a single period and verify alignment.
    
    Returns:
        (success, mean_offset, num_timestamps)
    """
    log(f"\n--- Testing pulse period={period}s ---")
    
    # Enable periodic output
    exit_code, _ = run_sdp_command([
        "-o",
        "--period", str(period),
        "--pin", output_pin,
        output_iface
    ])
    if exit_code != 0:
        log(f"Failed to enable output", to_stderr=True)
        return False, 0.0, 0
    
    # Verify output pin has perout function
    if not verify_pin_function(output_iface, output_pin, "perout"):
        return False, 0.0, 0
    
    # Capture timestamps - need enough time to get multiple edges
    # For 50% duty cycle, edges occur every period/2
    # Capture for at least 3 full periods plus overhead
    capture_time = max(3.5, period * 3 + 0.5)
    exit_code, timestamps_data = run_sdp_command([
        "-i",
        "-t", str(capture_time),
        "--pin", input_pin,
        input_iface
    ], quiet=False)  # Changed to False to see output in log
    
    # Verify input pin has extts function after capture command
    if not verify_pin_function(input_iface, input_pin, "extts"):
        # Disable output before returning
        run_sdp_command([
            "-o",
            "--period", "0",
            "--pin", output_pin,
            output_iface
        ])
        return False, 0.0, 0
    
    # Disable output
    run_sdp_command([
        "-o",
        "--period", "0",
        "--pin", output_pin,
        output_iface
    ])
    
    if exit_code == 2:  # Timeout with no data
        log("No timestamps received (exit code 2)", to_stderr=True)
        return False, 0.0, 0
    elif exit_code != 0:
        log(f"Failed to capture timestamps (exit code {exit_code})", to_stderr=True)
        return False, 0.0, 0
    
    # Extract timestamps from parsed JSON
    log(f"Received {len(timestamps_data)} timestamp records")
    timestamps = []
    for ts_data in timestamps_data:
        ts = ts_data.get("timestamp")
        if ts:
            timestamps.append(Decimal(str(ts)))
            log(f"    Got timestamp: {ts}")
    
    if not timestamps:
        log("No timestamps captured", to_stderr=True)
        return False, 0.0, 0
    
    # For 50% duty cycle signals, we capture both rising and falling edges
    # We get edges every period/2, so we should check all edges
    num_timestamps = len(timestamps)
    log(f"Captured {num_timestamps} edges")
    
    # Calculate offsets from expected positions
    offsets: List[float] = []
    for ts in timestamps:
        # Get fractional second
        frac_sec = float(ts % Decimal("1.0"))
        
        # Find closest expected alignment and calculate offset
        min_offset = 1.0
        for expected in expected_alignments:
            # Calculate offset considering wrap-around at 1.0 second boundary
            offset = frac_sec - expected
            # Handle wrap-around: if expected is 0.0 and we're near 1.0
            if expected == 0.0 and frac_sec > 0.9:
                offset = frac_sec - 1.0
            # Handle wrap-around: if expected is near 1.0 and we're near 0.0
            elif expected > 0.9 and frac_sec < 0.1:
                offset = frac_sec + 1.0 - expected
            
            if abs(offset) < abs(min_offset):
                min_offset = offset
        
        offsets.append(min_offset)
    
    # Calculate statistics
    mean_offset = sum(offsets) / len(offsets) if offsets else 0.0
    mean_abs_offset = sum(abs(o) for o in offsets) / len(offsets) if offsets else 0.0
    
    # Check if within tolerance
    success = mean_abs_offset <= ALIGNMENT_TOLERANCE
    
    # Log details
    if success:
        log(f"PASS: Mean offset={mean_offset*1000:.3f}ms (within {ALIGNMENT_TOLERANCE*1000:.0f}ms tolerance)")
    else:
        log(f"FAIL: Mean absolute offset={mean_abs_offset*1000:.3f}ms exceeds {ALIGNMENT_TOLERANCE*1000:.0f}ms tolerance")
        log(f"  Mean offset={mean_offset*1000:.3f}ms")
        # Show some sample offsets for debugging
        sample_offsets = offsets[:5]
        log(f"  Sample offsets (ms): {[f'{o*1000:.3f}' for o in sample_offsets]}")
    
    return success, mean_offset, num_timestamps


def get_pin_function(interface: str, pin: str) -> Optional[PinFunction]:
    """Get the current function of a pin using satpulsetool.
    
    Returns:
        'none', 'extts', 'perout', or None on error
    """
    exit_code, pins = run_sdp_command(["--pin", pin, interface], quiet=True)
    
    if exit_code != 0:
        log(f"Failed to get pin status")
        return None
    
    if pins:
        func = cast(PinFunction, pins[0].get("function", "none"))
        log(f"  Pin {interface}:{pin} function: {func}")
        return func
    
    return None


def verify_pin_function(interface: str, pin: str, expected_func: PinFunction, fail_on_error: bool = True) -> bool:
    """Verify a pin has the expected function.
    
    Args:
        interface: Interface name
        pin: Pin name or index
        expected_func: Expected function ('none', 'extts', 'perout')
        fail_on_error: If True, log as ERROR, otherwise WARNING
        
    Returns:
        True if pin has expected function, False otherwise
    """
    actual_func = get_pin_function(interface, pin)
    if actual_func == expected_func:
        log(f"  Pin {interface}:{pin} has expected function '{expected_func}'")
        return True
    else:
        msg = f"Pin {interface}:{pin} has function '{actual_func}', expected '{expected_func}'"
        if fail_on_error:
            log(f"  ERROR: {msg}", to_stderr=True)
        else:
            log(f"  WARNING: {msg}")
        return False


def main() -> None:
    global satpulsetool, LOG_FILE
    
    if len(sys.argv) != 6:
        fatal(__doc__, 1)
    
    satpulsetool = sys.argv[1]
    output_iface = sys.argv[2]
    output_pin = sys.argv[3]
    input_iface = sys.argv[4]
    input_pin = sys.argv[5]
    
    # Check if satpulsetool exists
    if not os.path.isfile(satpulsetool):
        print(f"Error: satpulsetool not found at {satpulsetool}", file=sys.stderr)
        sys.exit(1)
    
    # Open log file
    log_filename = f"sdp-loopback-{datetime.now().strftime('%Y%m%d-%H%M%S')}.log"
    LOG_FILE = open(log_filename, 'w')
    
    log(f"SDP Loopback Test: {output_iface}:{output_pin} -> {input_iface}:{input_pin}", to_stderr=True)
    log(f"Log file: {log_filename}", to_stderr=True)
    log("", to_stderr=True)
    
    log(f"Testing SDP loopback: {output_iface}:{output_pin} -> {input_iface}:{input_pin}")
    
    # Check if running as root (needed for PHC operations)
    if os.geteuid() != 0:
        log("Warning: Not running as root. Some operations may fail.", to_stderr=True)

    def require_interface(name: str) -> InterfaceInfo:
        info = get_interface_info(name)
        if not info:
            fatal(f"FAIL Interface {name} not found or has no PHC")
            # Type narrowing - after fatal(), info cannot be None
            assert False, "unreachable"
        status = info.get("status", "unknown")
        if "down" in status.lower():
            fatal(f"FAIL Interface {name} is down (status: {status})\n  Please bring it up: sudo ip link set {name} up")
        driver = info.get("driver", "")
        if driver not in ["igb", "igc"]:
            fatal(f"FAIL Interface {name} uses driver '{driver}', not igb/igc")
        log(f"Interface {name}: {status}, driver={driver}")
        return info

    # Check drivers and interface status
    log("Checking interfaces...", to_stderr=True)
    require_interface(input_iface)
    require_interface(output_iface)
    
    # Discovery phase
    log("\n=== Interface Discovery ===")
    run_sdp_command(["--show"])
    run_sdp_command(["--show", input_iface])
    if output_iface != input_iface:
        run_sdp_command(["--show", output_iface])
    
    log(f"Using pins: input={input_iface}:{input_pin}, output={output_iface}:{output_pin}")
    
    # Sync PHC clocks
    log("Syncing PHC clocks...", to_stderr=True)
    sync_phc_clocks(input_iface, output_iface)
    
    # Run period tests
    log("\nPeriod Tests:", to_stderr=True)
    
    all_passed: bool = True
    first_test: bool = True
    
    for period, alignments in TEST_PERIODS:
        log(f"\nTesting pulse period {period}s:", to_stderr=True)
        
        passed, mean_offset, num_timestamps = test_period(
            input_iface, input_pin, 
            output_iface, output_pin,
            period, alignments
        )
        
        all_passed = all_passed and passed
        
        # Print results to stderr
        if passed:
            log(f"  PASS: Received {num_timestamps} timestamps. Mean offset {mean_offset*1000:.3f}ms", to_stderr=True)
        else:
            log(f"  FAIL: Received {num_timestamps} timestamps. Mean offset {mean_offset*1000:.3f}ms (tolerance {ALIGNMENT_TOLERANCE*1000:.0f}ms exceeded)", to_stderr=True)
        
        # If 1.0s period test (first test) fails, stop immediately
        if first_test and not passed:
            log("\n=== 1.0s period test failed - stopping ===")
            fatal(
                "\n*** 1.0s period test failed - no connectivity ***\n" \
                "Check:\n" \
                "  1. Cable is connected between pins\n" \
                f"  2. Output pin {output_iface}:{output_pin} can generate output\n" \
                f"  3. Input pin {input_iface}:{input_pin} can receive input"
            )
        first_test = False
    
    # Disable test
    log("\nTesting pin disable functionality:", to_stderr=True)
    log("\n=== Disable Test ===")
    
    # Disable output pin
    exit_code, _ = run_sdp_command([
        "--disable",
        "--pin", output_pin,
        output_iface
    ])
    if exit_code != 0:
        log(f"Failed to disable output pin")
        all_passed = False
    
    # Disable input pin
    exit_code, _ = run_sdp_command([
        "--disable",
        "--pin", input_pin,
        input_iface
    ])
    if exit_code != 0:
        log(f"Failed to disable input pin")
        all_passed = False
    
    # Verify pins are disabled
    log("Verifying pins are disabled:")
    disable_out_ok = verify_pin_function(output_iface, output_pin, "none")
    disable_in_ok = verify_pin_function(input_iface, input_pin, "none")
    
    if disable_out_ok and disable_in_ok:
        print("  PASS: Pins disabled correctly", file=sys.stderr)
    else:
        print("  FAIL: Pin disable verification failed", file=sys.stderr)
        all_passed = False
    
    # Final result
    log("", to_stderr=True)
    if all_passed:
        log("\nAll tests PASSED", to_stderr=True)
        log("\n=== All tests PASSED ===")
    else:
        log("\nSome tests FAILED", to_stderr=True)
        log("\n=== Some tests FAILED ===")
    
    LOG_FILE.close()
    sys.exit(0 if all_passed else 1)


if __name__ == "__main__":
    main()
