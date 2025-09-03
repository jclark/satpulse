
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
                 [-o|--perout] [-w|--width seconds] [--period seconds] [--phase seconds]
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

**-o**, **--perout**  
Enable periodic output. If **-w/--width** is specified, sets duty cycle control; otherwise generates a continuous square wave. Period defaults to 1.0 seconds (PPS).

**-w**, **--width** *seconds*  
Set the pulse width for periodic output in seconds. Must be greater than 0 and less than the period. If not specified, no duty cycle control is applied. Cannot be explicitly set to 0.

**--disable**  
Disable the pin function (sets pin to function 0).

**--show**  
Show the current configuration of the PHC including capabilities, pin assignments, and interface status. This is the default mode when no other mode is specified.

### Options for --extts

**-t**, **--timeout** *seconds*  
How long to monitor for input events in seconds (default: 2.0). Use 0 for unlimited (monitor until interrupted).

**--show-stale**  
Include stale/buffered timestamps in output. By default, stale timestamps (those buffered in the kernel before monitoring started) are skipped. With this flag, they are included with `"stale": true` in JSON output.

### Options for --perout

**-w**, **--width** *seconds*  
Set the pulse width for periodic output in seconds. Must be greater than 0 and less than the period. If not specified, no duty cycle control is applied. Cannot be used when period is 0.

**--period** *seconds*  
Set the period for periodic output in seconds (default: 1.0). A period of 0 disables the output signal.

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
satpulsetool sdp -o -w 0.1 eth0

# Enable PPS output with no duty cycle control (square wave)
satpulsetool sdp -o eth0

# Enable 10Hz output with 10ms pulse width
satpulsetool sdp -o -w 0.01 --period 0.1 eth0

# Disable periodic output
satpulsetool sdp -o --period 0 eth0

# Disable pin 2
satpulsetool sdp --disable --pin 2 eth0

# Monitor on channel 1
satpulsetool sdp -i --chan 1 eth0

# Output in JSON format
satpulsetool sdp --jsonl eth0
```

## Output conventions
- All data output (interface info, pin status, timestamps) goes to stdout
- All errors, warnings, and informational messages go to stderr  
- All and only stdout output is affected by the `-j`/`--jsonl` flag
- This allows proper piping and redirection (e.g., `satpulsetool sdp -i eth0 | tee timestamps.log`)


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
   - If no interfaces have pins, print error to stderr and exit with status 2
   - Output as text or JSON to stdout based on `--jsonl` flag
   - **Note**: Consider reimplementing without sysfs since PHC ioctls don't require root for reading

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

### Phase 3: External timestamps mode (`-i`)

#### Architecture
1. **Mode-based flag parsing in `sdpflags.go`**:
   - Add Mode enum: `ModeShowAll`, `ModeShow`, `ModeExtts`, `ModePerout`, `ModeDisable`
   - Create `FlagConfig` struct with Mode field and mode-specific fields
   - Parse `-i`/`--extts`, `-t`/`--timeout`, `--show-stale`, `--pin`, `--chan` flags
   - Determine mode from flags (interface only → show, -i → extts, etc.)
   - Validate only relevant flags are set for each mode
   - Default timeout to 2.0 seconds, 0 for unlimited

2. **Mode dispatch in `sdpcmd.go`**:
   - Create handler map: `map[Mode]func(*FlagConfig) ([]Printer, error)`
   - Panic on internal errors (e.g., unknown mode)
   - Keep existing uniform output handling

3. **Create `extts.go`** with timestamp monitoring:
   - Define `ExttsEvent` struct implementing `Printer`
   - Main `extts()` function opens device, configures pin, starts worker
   - Worker goroutine pattern based on `internal/ts/clock.go:readWorker()`

#### Implementation Details

**ExttsEvent struct**:
```go
type ExttsEvent struct {
    Timestamp float64 `json:"timestamp"` // seconds since epoch
    Index     uint32  `json:"index"`     // channel index  
    Stale     bool    `json:"stale,omitempty"` // true if buffered
}

func (e *ExttsEvent) Print(f *os.File) {
    fmt.Fprintf(f, "%.9f\n", e.Timestamp)
}
```

**Worker goroutine pattern** (based on `internal/ts/clock.go`):
- Run `ExttsAvailable(100ms)` in a loop
- Track stale state: starts true, becomes false after timeout with no events
- Read timestamps with `ReadExtts()` when available
- Send events on channel to main function
- Handle context cancellation for cleanup

**Main function pattern**:
```go
func extts(cfg *FlagConfig) ([]Printer, error) {
    // 1. Open PHC device using phc.IfPhcIndex() and phc.Open()
    // 2. Resolve pin name to index if needed (try parse as int first)
    // 3. Configure pin: phc.PinSetFunc(pin, phc.PinFuncExtts, chan)
    // 4. Enable timestamps: phc.ExttsEnable(chan, true)
    
    ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout)
    defer cancel()
    defer phc.ExttsEnable(chan, false) // cleanup
    
    eventCh := make(chan ExttsEvent, 10)
    go exttsWorker(ctx, clk, eventCh, cfg.Channel)
    
    var events []Printer
    for {
        select {
        case event := <-eventCh:
            if !event.Stale || cfg.ShowStale {
                events = append(events, &event)
                // Could also print immediately for streaming
            }
        case <-ctx.Done():
            return events, nil
        }
    }
}
```

**Pin name resolution**:
- Try `strconv.Atoi()` first
- If not a number, iterate pins 0 to n_pins-1 with `PinGetFunc()`
- Compare name field, use matching index

**Error messages**:
- "interface %s does not have a PTP hardware clock"
- "interface %s has no software-defined pins"  
- "pin %s not found on interface %s"
- "channel %d out of range (max %d)"

#### Key differences from `internal/ts/clock.go`:
- No era tracking (just simple stale boolean)
- No carrier loss handling
- No interface waiting logic
- Simpler event structure (just timestamp, index, stale)
- Output directly as ExttsEvent instead of complex Event type

### Phase 4: Periodic output mode (`-o`)
1. Extend `sdpflags.go` to parse `-o`/`--perout`, `-w`/`--width`, `--period`, `--phase`, `--pin`, `--chan` options
2. Validate mutual exclusivity with `-i` mode
3. Validate `--width` if specified (must be > 0 and < period, cannot be explicitly 0)
4. Open PHC device (requires root)
5. Parse `--pin` (handle both names and indices)
6. Validate pin and channel indices against capabilities
7. Convert float seconds to `PtpClockTime`
8. Use `golang.org/x/sys/unix` directly:
   - `unix.IoctlPtpPinSetfunc()` to configure pin for perout
   - `unix.IoctlPtpPeroutRequest2()` to set period/phase
   - If width specified (> 0): set PTP_PEROUT_DUTY_CYCLE flag and width
   - If width not specified (= 0): don't set duty cycle flag
9. No output in this mode (configuration only)

### Phase 5: Disable mode (`--disable`)
1. Extend `sdpflags.go` to parse `--disable` and `--pin` options
2. Validate mutual exclusivity with other modes
3. Open PHC device (requires root)
4. Parse `--pin` (handle both names and indices)
5. Use `unix.IoctlPtpPinSetfunc()` to set function to PTP_PF_NONE
6. No output in this mode

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
    Stale     bool    `json:"stale,omitempty"` // true if timestamp was buffered before monitoring started
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

### Exit codes
The tool uses the following exit codes:
- **0**: Success - operation completed normally
- **1**: Error - any error condition (invalid flags, device errors, permission errors, etc.)
- **2**: No data - operation succeeded but no data was available:
  - `showAll` mode when no interfaces with PHC and pins are detected
  - `extts` mode when no timestamps are received during the monitoring period
  
This allows scripts to distinguish between actual errors and the absence of expected data.

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
├── sdpcmd.go       # Main command entry point with mode dispatch
├── sdpflags.go     # Flag parsing and mode determination
├── showall.go      # List all interfaces with PHC/pins
├── show.go         # Show specific interface configuration
├── extts.go        # External timestamp monitoring (-i mode)
├── perout.go       # Periodic output configuration (-o mode)
└── disable.go      # Disable pin function (--disable mode)
```

### Architecture Design

#### Flag Processing
The `sdpflags.go` should:
1. Parse all command-line flags
2. Determine the mode based on flags:
   - No interface specified → `showAll`
   - Interface specified, no mode flags → `show`
   - `-i`/`--extts` flag → `extts`
   - `-o`/`--perout` flag → `perout`
   - `--disable` flag → `disable`
3. Validate that only relevant flags are set for each mode
4. Return a struct with:
   - `Mode` field indicating which mode
   - Only the validated fields relevant to that mode

#### Mode Dispatch
The `sdpcmd.go` should:
1. Define a common handler signature: `func(flags *FlagConfig) ([]Printer, error)`
2. Create a map of mode to handler:
   ```go
   modeHandlers := map[Mode]func(*FlagConfig) ([]Printer, error){
       ModeShowAll: showAll,
       ModeShow:    show,
       ModeExtts:   extts,
       ModePerout:  perout,
       ModeDisable: disable,
   }
   ```
3. Call the appropriate handler based on the mode
4. Handle the uniform output (text/JSON) for all modes

#### Mode Implementation
Each mode file implements its handler function:
- Takes the validated flag config
- Performs the mode-specific operation
- Returns `[]Printer` for output (or empty/nil for no output)
- Returns error on failure

## Implementation Status and Next Steps

### Current Status
- **Phase 1 (showAll)** - COMPLETE in `showall.go` - Lists all interfaces with PHC/pins
- **Phase 2 (show)** - COMPLETE in `show.go` - Shows specific interface configuration  
- **Phase 3 (extts)** - TO BE IMPLEMENTED - External timestamp monitoring (`-i` mode)
- **Phase 4 (perout)** - Not started - Periodic output (`-o` mode)
- **Phase 5 (disable)** - Not started - Disable pin function (`--disable` mode)

### Files to modify/create for Phase 3:
1. **`sdpflags.go`** - Add Mode type, FlagConfig struct, full flag parsing
2. **`extts.go`** - New file with extts handler and worker goroutine
3. **`sdpcmd.go`** - Add mode dispatch map

### Next Session Implementation
Start with Phase 3 implementation using the detailed plan above

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

4. **System testing**: Integrate `satpulsetool sdp` into the Ansible system testing approach in `systest/`

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



