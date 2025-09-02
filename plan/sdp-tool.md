# SDP tool - Software defined pin control for PTP hardware clocks

## Problem

Currently, users who need to control software-defined pins on PTP hardware clocks must use `tools/testing/selftests/ptp/testptp.c` from the Linux kernel, which has several problems:
- Must be extracted from the kernel source tree and compiled manually
- Requires matching kernel headers and running kernel versions
- Overall design of command is not ergonomic, especially for users that are not kernel developers
- Error messages aren't as helpful as they could be

## Solution

Create a new `sdp` subcommand for `satpulsetool` that focuses specifically on controlling software-defined pins. This complements the `phc_ctl` command from linuxptp.

Unlike `testptp`, which has grown by accretion of features, this tool is designed as a cohesive whole with specific use cases in mind, mostly importantly
1. Checking whether PPS input is working
2. Enabling PPS output

The result is a tool where simple operations have simple syntax - for example, `satpulsetool sdp -i eth0` checks for PPS input.

## Current manual approach vs new tool

### Current approach using sysfs
Currently, users must manually configure PPS input using multiple sysfs writes:

```bash
# Configure pin for input (assuming ptp0 and SYNC_OUT pin)
echo 1 0 | sudo tee /sys/class/ptp/ptp0/pins/SYNC_OUT
# Where: 1 = extts function, 0 = channel 0

# Enable timestamping on channel 0
echo 0 1 | sudo tee /sys/class/ptp/ptp0/extts_enable
# Where: 0 = channel 0, 1 = enable

# Read timestamps
sudo cat /sys/class/ptp/ptp0/fifo
# Output: channel_number seconds nanoseconds
```

Problems with this approach:
- Requires knowledge of sysfs paths and format
- Must know the PTP device number (ptp0, ptp1, etc.)
- Must know the exact pin name
- Multiple steps with cryptic syntax
- No helpful error messages
- Must manually disable when done

### New approach with sdp tool
```bash
# Simple one-command PPS input test
satpulsetool sdp -i eth0

# Or with specific pin name
satpulsetool sdp -i --pin SYNC_OUT eth0
```

Benefits:
- Single command
- Uses interface name instead of PTP device number
- Automatic configuration and cleanup
- Clear error messages
- Handles interface state checking

## User scenarios

### Primary use cases
1. **Check whether PPS input is working** - Check if an interface is receiving PPS input signals (requires interface up); should give warnings for things that might cause it not to working (e.g. interface in wrong state)
2. **Enable PPS output** - Configure a pin to generate PPS output signals

### Secondary use cases
- List pin capabilities and interface status
- Query pin current configuration
- Configure pin functions directly (none, extts, perout)
- Configure periodic output with custom period/phase

## Synopsis

```
satpulsetool sdp [-h|--help] [--jsonl]
                 [-i|--extts] [-t|--timeout seconds]
                 [-o|--perout width] [--period seconds] [--phase seconds]
                 [--disable]
                 [--pin n] [--chan n]
                 [interface]
```

### Command design principles

Consistent look and feel with `satpulsetool gps` and the rest of the SatPulse suite:
1. **Positional argument**: Interface name comes last (e.g., `eth0`)
2. **Option precedence**: Options before positional arguments  
3. **Short options**: For frequently used operations (-i, -o, -t)
4. **Long options**: More descriptive alternatives (--extts, --perout, --timeout)
5. **Interface names** instead of `/dev/ptpX` numbers - stable and meaningful to users
6. **Time values** as float seconds (e.g., 0.1 for 100ms, 1e-6 for 1μs, 1e-9 for 1ns)
7. **Clear diagnostics** when operations fail (interface down, no carrier, etc.)
8. **JSON output** via --jsonl flag for scripting (--show and -i modes)
9. **Help**: Both -h and --help supported

## Options

### General options

**-h**, **--help**  
Show usage help for the sdp command.

**-j**, **--jsonl**  
Output in JSON lines format instead of human-readable text.

### Mode options (mutually exclusive)

**-i**, **--extts**  
Monitor external timestamp events (PPS input). Automatically configures the pin for external timestamping, enables it, and reads events. **Important**: The interface must be up for external timestamping to work.

**-o**, **--perout** *width*  
Enable periodic output with the specified pulse width in seconds. The width must be >= 0 and < 1.0. A width of 0 disables output. Period defaults to 1.0 seconds (PPS).

**--disable**  
Disable the pin function (sets pin to function 0).

**--show**  
Show the current configuration of the PHC including capabilities, pin assignments, and interface status. This is the default mode when no other mode is specified.

### Options for --extts

**-t**, **--timeout** *seconds*  
How long to monitor for input events in seconds (default: 2.0). Use 0 for unlimited (monitor until interrupted).

### Options for --perout

**--period** *seconds*  
Set the period for periodic output in seconds (default: 1.0).

**--phase** *seconds*  
Set the phase offset for periodic output in seconds.

### Common options

**--pin** *n*  
Specify which pin to use (default: 0). Can be either a pin index (e.g., `0`, `1`) or a pin name (e.g., `SYNC_OUT`). When using a name, the tool will iterate through pins using `PTP_PIN_GETFUNC` to find the matching pin index. Applies to all modes.

**--chan** *n*  
Specify which channel to use (default: 0). Must be within the valid range for the operation (0 to n_extts_channels-1 for extts, 0 to n_perout_channels-1 for perout). Applies to input and output modes.

## Examples

```bash
# List all interfaces with PHC (no root required)
satpulsetool sdp

# Show PHC configuration for eth0
satpulsetool sdp eth0

# Monitor for PPS input on eth0 (2 second timeout)
satpulsetool sdp -i eth0

# Monitor for PPS input continuously
satpulsetool sdp -i -t 0 eth0

# Enable PPS output with 100ms pulse width
satpulsetool sdp -o 0.1 eth0

# Enable 10Hz output with 10ms pulse width
satpulsetool sdp -o 0.01 --period 0.1 eth0

# Disable output
satpulsetool sdp -o 0 eth0

# Disable pin 2
satpulsetool sdp --disable --pin 2 eth0

# Monitor on channel 1
satpulsetool sdp -i --chan 1 eth0

# Output in JSON format
satpulsetool sdp --jsonl eth0
```



## Implementation plan

### Phase 1: Show mode without interface (default)
1. Create `internal/sdpcmd/` package with `sdpcmd.go`
2. Add sdp command registration in `cmd/satpulsetool/satpulsetool.go`
3. Create minimal `sdpflags.go` with just `-j`/`--jsonl` and help flags using pflag
4. Define `InterfaceInfo` struct for output
5. Parse flags to check for `--jsonl` and help
6. If no positional args after flag parsing, implement list mode:
   - List all network interfaces
   - For each interface, use `phc.IfPhcIndex()` to check for PHC (no root required)
   - Skip interface if PHC index < 0 (no PHC)
   - For interfaces with PHC, read from `/sys/class/ptp/ptpX/`:
     - `n_pins` or `n_programmable_pins` - Number of pins (try both for kernel compatibility)
     - Skip interface if n_pins is 0 (no software-defined pins)
     - `clock_name` - Hardware clock name
     - `n_external_timestamps` - Number of extts channels
     - `n_periodic_outputs` - Number of perout channels
     - List files in `pins/` directory to get pin names (directory won't exist if n_pins=0)
   - Only include interfaces with PHC and n_pins > 0 in results
   - If no interfaces have pins, print error to stderr and exit with status 1
   - Output as text or JSON based on `--jsonl` flag

### Phase 2: Show mode with interface specified
1. Check if positional arg exists after flag parsing - that's the interface
2. Get PHC index using `phc.IfPhcIndex()`
3. Check `/sys/class/ptp/ptpX/n_pins` or `n_programmable_pins`
4. If n_pins is 0, error: "interface %s has no software-defined pins"
5. Define `PinStatus` struct when needed
6. If running as root:
   - Open PHC device with `phc.Open()`
   - Query capabilities with `PTP_CLOCK_GETCAPS`
   - Iterate through pins with `PTP_PIN_GETFUNC`
   - Output full `PinStatus` structs with indices
7. If not root:
   - Read from `/sys/class/ptp/ptpX/pins/` directory
   - Parse function and channel from each pin file
   - Output `PinStatus` without index field
8. Use `--jsonl` flag from Phase 1 parsing for output format

### Phase 3: Flag parsing infrastructure
1. Extend `sdpflags.go` from Phase 1
2. Define mode flags: `-i`, `-o`, `--disable`, `--show`
3. Parse common options: `--pin`, `--chan`
4. Validate mutual exclusivity of mode flags
5. Return parsed configuration struct

### Phase 4: External timestamps mode (`-i`)
1. Define `ExttsEvent` struct
2. Create `sdpphc.go` for PHC helper functions
3. Open PHC device (requires root)
4. Implement pin name resolution if `--pin` is a string
5. Validate pin and channel indices against capabilities
6. Use existing `phc` package functions:
   - `phc.PinSetFunc()` to configure pin for extts
   - `phc.ExttsEnable()` to enable timestamps
   - `phc.ExttsAvailable()` and `phc.ReadExtts()` in loop
7. Output stream of `ExttsEvent` structs
8. Clean up on exit (disable timestamps)

### Phase 5: Periodic output mode (`-o`)
Parse `-o`/`--perout`, `--period`, `--phase`, `--pin`, `--chan` options:
- Open PHC device (requires root)
- Parse `--pin` (handle both names and indices)
- Validate pin and channel indices against capabilities
- Convert float seconds to `PtpClockTime`
- Use `golang.org/x/sys/unix` directly:
  - `unix.IoctlPtpPinSetfunc()` to configure pin for perout
  - `unix.IoctlPtpPeroutRequest()` to set period/width/phase
- Handle width=0 as disable
- No output in this mode (configuration only)

### Phase 6: Disable mode (`--disable`)
Parse `--disable` and `--pin` options:
- Open PHC device (requires root)
- Parse `--pin` (handle both names and indices)
- Use `unix.IoctlPtpPinSetfunc()` to set function to PTP_PF_NONE
- No output in this mode

## Technical Details

### Output architecture

All operations that produce output follow a consistent pattern:
1. Generate output as a list of structs
2. Each struct represents one line of output
3. Based on `--jsonl` flag, either:
   - Marshal each struct to JSON and output one per line (JSON lines format)
   - Format structs as human-readable text

This applies to:
- Show mode (list of interface/pin info structs)
- Input monitoring (stream of timestamp event structs)

### Output struct definitions

```go
// InterfaceInfo represents a network interface with PHC (for list mode)
type InterfaceInfo struct {
    Name             string   `json:"name"`
    ClockIndex       int      `json:"clock_index"`       // PHC index from ethtool
    ClockName        string   `json:"clock_name"`        // From /sys/class/ptp/ptpX/clock_name
    NumPins          int      `json:"n_pins"`            // From /sys/class/ptp/ptpX/n_pins
    NumExttsChannels int      `json:"n_extts_channels"`  // From /sys/class/ptp/ptpX/n_external_timestamps
    NumPeroutChannels int     `json:"n_perout_channels"` // From /sys/class/ptp/ptpX/n_periodic_outputs
    PinNames         []string `json:"pin_names"`         // From /sys/class/ptp/ptpX/pins/ directory listing
}

// PinStatus represents a single pin's configuration (for show with interface)
type PinStatus struct {
    Index    uint32 `json:"index"`
    Name     string `json:"name"`
    Function string `json:"function"` // "none", "extts", "perout", "physync"
    Channel  uint32 `json:"channel"`
}

// ExttsEvent represents a timestamp event (for -i mode)
type ExttsEvent struct {
    Timestamp float64 `json:"timestamp"` // seconds since epoch
    Index     uint32  `json:"index"`     // channel index
}
```

Note: `physync` stands for physical layer synchronization, used in some advanced PTP configurations for synchronizing the physical layer timing.

### Implementation approach

#### Package usage
- **For extts (input monitoring)**: Use existing `internal/phc` package which already provides:
  - `phc.IfPhcIndex()` - Get PHC index from interface name
  - `phc.Open()` - Open the PHC device
  - `phc.PinSetFunc()` - Configure pin function
  - `phc.ExttsEnable()` - Enable/disable external timestamps
  - `phc.ExttsAvailable()` - Poll for events
  - `phc.ReadExtts()` - Read timestamp events

- **For perout (periodic output)**: Use `golang.org/x/sys/unix` directly:
  - `unix.IoctlPtpClockGetcaps()` - Query capabilities
  - `unix.IoctlPtpPinGetfunc()` - Query pin configuration
  - `unix.IoctlPtpPinSetfunc()` - Set pin function
  - `unix.IoctlPtpPeroutRequest()` - Configure periodic output

#### Integration notes
- `sdpcmd.go` will handle command entry and coordinate operations
- `sdpflags.go` will parse flags similar to `gpscmd/gpsflags.go`
- `sdpphc.go` will contain only perout-specific code and helper functions
- Extts operations will primarily delegate to `internal/phc` package
- Both modes will share common PHC device opening logic

#### Pin name resolution
When `--pin` is given a string instead of a number:
1. Open the PHC device
2. Query capabilities to get n_pins
3. Iterate through pins 0 to n_pins-1 using `PTP_PIN_GETFUNC`
4. Compare the returned pin name with the user-provided name
5. Use the matching index, or error if no match found
This requires root privileges since `PTP_PIN_GETFUNC` needs an open PHC device.

#### Sysfs operations (non-root)
For the list mode without root privileges, read from sysfs:
- `/sys/class/ptp/ptpX/clock_name` - Hardware clock name (string)
- `/sys/class/ptp/ptpX/n_pins` or `/sys/class/ptp/ptpX/n_programmable_pins` - Number of programmable pins (integer) - try both names for kernel compatibility
- `/sys/class/ptp/ptpX/n_external_timestamps` - Number of extts channels (integer)
- `/sys/class/ptp/ptpX/n_periodic_outputs` - Number of perout channels (integer)
- `/sys/class/ptp/ptpX/pins/` - Directory containing pin files (only exists if n_pins > 0)

These files are world-readable and provide basic PHC information without requiring root.

### Error handling
- If no interfaces have software-defined pins: "no interfaces with software-defined pins found"
- If specified interface has no PHC: "interface %s does not have a PTP hardware clock"
- If specified interface has no pins: "interface %s has no software-defined pins"
All errors should be printed to stderr and cause non-zero exit status.

#### Time conversion
- User provides time values as float64 seconds
- Convert to `unix.PtpClockTime` for perout:
  ```go
  func secondsToPtpClockTime(seconds float64) unix.PtpClockTime {
      sec := int64(seconds)
      nsec := uint32((seconds - float64(sec)) * 1e9)
      return unix.PtpClockTime{Sec: sec, Nsec: nsec}
  }
  ```
- Convert from `ptime.Time` for extts display:
  ```go
  func ptimeToSeconds(t ptime.Time) float64 {
      return float64(t) / 1e9
  }
  ```

### Package Structure

```
internal/sdpcmd/
├── sdpcmd.go       # Main command entry point
├── sdpflags.go     # Flag parsing and validation
└── sdpphc.go       # PHC operations and ioctl wrappers
```

## Implementation Status and Next Steps

### Current Status
- Plan document complete with 6 implementation phases
- Key design decisions made:
  - Use interface names instead of /dev/ptpX
  - Support pin names as well as indices
  - Separate root and non-root capabilities
  - Filter to only show interfaces with software-defined pins

### Ready to Implement
Start with **Phase 1: Show mode without interface**
- This is the simplest functionality requiring no root
- Tests basic sysfs reading and output formatting
- Provides immediate value

### Testing Requirements
For testing the implementation, you'll need:
1. **Hardware with PHC and software-defined pins**
   - Raspberry Pi CM4 (has 1 pin)
   - Intel i210/i225 NICs (have 4 pins typically)
   - Other server NICs with IEEE 1588 support

2. **Test scenarios to verify**:
   - List mode filters out interfaces without pins
   - Show mode works with/without root
   - Pin name resolution works correctly
   - Extts monitoring receives events
   - Perout configuration generates output signal

3. **Kernel compatibility**:
   - Test with kernels using `n_pins` vs `n_programmable_pins`
   - Verify sysfs paths exist as expected

### Key Implementation Details from Discussion
1. **Phase ordering**: Implement by major modes, not by infrastructure layers
2. **Struct definitions**: Define only when needed (except InterfaceInfo in Phase 1)
3. **Flag parsing**: Start minimal, extend as needed
4. **Error handling**: All errors to stderr, non-zero exit
5. **JSON output**: Use `-j`/`--jsonl` flag from Phase 1

### Files to Reference During Implementation
- `internal/phc/phc.go` - Existing PHC functions for extts
- `internal/gpscmd/` - Pattern for command structure
- `golang.org/x/sys/unix` package - PTP ioctl wrappers (IoctlPtpPeroutRequest, etc.)
- `tmp/ptp_clock.h` - Kernel PTP structures for reference
- `tmp/testptp.c` - The kernel test utility we're replacing, shows:
  - How to configure pins (lines 404-414, `-L` option)
  - How to enable/disable extts (lines 416-441, `-e` option)
  - How to configure perout (lines 475-504, `-p` option with `-w` and `-H`)
  - How to list pins (lines 447-463, `-l` option)



