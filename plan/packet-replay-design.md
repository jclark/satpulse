# Packet Replay Testing Design

## Problem

Need a robust regression testing strategy for GPS configuration logic that doesn't require real hardware. The current `ubxcfg.go` configuration system is complex and has many branches depending on GPS firmware versions, capabilities, and responses. Manual testing with real GPS units is slow and hardware-dependent.

## Solution

Create a packet replay testing system that:
1. Records real GPS configuration sessions using `satpulsetool gps` to log packets
2. Replays the packet sequences to verify configuration logic produces identical results
3. Validates both packet sequences and final configuration state

## Key Architectural Aspects

### Pure Configuration Logic
- `gpsprot/config.go` defines interfaces with no I/O dependencies
- `ubx/ubxcfg.go` implements pure, deterministic configuration logic
- Uses only timestamps from incoming packets (never `time.Now()`)
- ACK matching relies on `tSent` vs `tRead` timing from packets

### Request/Response Pattern
- Configurator receives packets via `ProcessPacket()`
- Returns packets to send via `NextRequest()`
- `FindAck()` matches acknowledgments to sent packets
- `Done()` confirms successful configuration steps

### Current Flow
```
gpsio → gpscfg → configurator → packets to send → gpsio
```

### Replay Flow
```
packet log → replay engine → configurator → verify packets match log
```

## Implementation Strategy

### Test Data Structure
```
internal/gpscmd/
  replay_test.go        # generated test file
  testdata/
    NNNN.jsonl          # test log (JSON lines format)
```

### File Format

**NNNN.jsonl**: Test log in JSON lines format combining all test data

Example structure:
```jsonl
{"type": "args", "args": ["gps", "--gnss", "gps", "--band", "L1", "--save"]}
{"type": "version", "version": "satpulse git version 20250522.67860f2, build date 2025-05-23 00:14:53+00:00", "hostname": "abondance.lan"}
{"packet": "b5620a04000e", "tRead": "2025-01-23T10:30:45.123Z", "out": true}
{"packet": "b5620a050014", "tRead": "2025-01-23T10:30:45.145Z"}
... more packet log entries (existing format unchanged) ...
{"type": "receiver", "model": "ZED-F9P", "firmware": "HPG 1.12", "protocol": "23.01", "module": "ZED-F9P"}
{"type": "config", "signalsEnabled": [["GAL", "E1"], ["GPS", "L1CA"]], "baudRate": 115200, "nmeaEnabled": false}
```

**Entry types:**
- **args**: Command-line arguments as passed to `satpulsetool gps`
- **version**: Satpulse version info (from `cmd.VersionInfo()`) and hostname
- **packet**: Existing packet log format (unchanged)
  - `out: true` = packet sent to GPS
  - `out: false/missing` = packet received from GPS
- **version**: GPS version/capability info (from `printVersion`)
- **config**: Selected final configuration properties
  - Only "interesting" properties (configured via command-line, key behavioral settings)
  - `signalsEnabled` as `[constellation, signal]` pairs
  - Avoids full ConfigProps dump to prevent test invalidation on internal changes

### Test Generation Strategy

1. **`--test-log` option**: Add to `satpulsetool gps` command
2. **Generated test file**: `replay_test.go` with explicit test functions
3. **Test verification points**:
   - Packet sequences match logged outgoing packets
   - Final `ConfigProps` matches logged config properties
   - Version info provides test context

### Replay Engine Logic

1. Parse test log file to extract args, packets, version, and config entries
2. Parse command-line args using `parseFlags()`
3. Create `ConfigTarget` using `createConfigTarget()`
4. Initialize configurator with target
5. Feed "in" packets to configurator (reusing packet log entries)
6. Call `NextRequest()` and verify returned packets match logged "out" packets
7. Handle ACKs by calling `FindAck()` and `Done()` appropriately
8. Verify final `ConfigProps` matches logged config properties

### Benefits

- **No hardware required**: Run comprehensive tests in CI/CD
- **Real-world scenarios**: Test data from actual GPS units
- **Regression detection**: Catch configuration logic changes
- **Multiple GPS models**: Accumulate test cases for different hardware
- **Debugging friendly**: Clear test names and expected outputs
- **Deterministic**: Pure configuration logic ensures reproducible results

### Test Data Collection Workflow

1. Run `satpulsetool gps --test-log NNNN.jsonl [other-args]` with real GPS
2. Manually verify configuration worked correctly  
3. Copy `NNNN.jsonl` to `testdata/`
4. Regenerate `replay_test.go` to include new test case

### Implementation Details

**`--test-log` implementation flow:**
1. Write `args` entry at start
2. Write `version` entry with satpulse version and hostname
3. Enable existing packet logging (code unchanged)
4. Run configuration process
5. Wait for packet logging goroutine to complete and flush
6. Write `receiver` entry (available after configuration)
7. Write `config` entry with selected final properties

**Key synchronization point**: Must ensure packet logging goroutine has terminated and flushed all packets before appending version and config entries.

## Implementation Staging

### Stage 1: Implement `--test-log` Option
- Add `--test-log` flag to `satpulsetool gps` command-line parsing
- Implement test log file creation with proper entry sequencing:
  1. Write `args` entry with command-line arguments
  2. Write `version` entry with `cmd.VersionInfo()` and hostname
  3. Enable existing packet logging (reuse current implementation)
  4. Run normal GPS configuration process
  5. Ensure packet logging goroutine completes and flushes
  6. Write `receiver` entry with GPS version/model info
  7. Write `config` entry with selected final configuration properties

### Stage 2: Generate Test Cases
- Use `--test-log` with real GPS hardware to create test cases:
  - Different GPS models (ZED-F9P, etc.)
  - Different firmware versions (legacy vs new UBX)
  - Various configuration scenarios (signals, bands, speeds, etc.)
  - Both successful and expected failure cases
- Manually verify each test case works as expected using `ubxanno` tool to decode packet payloads
- Build initial test suite in `testdata/` directory

### Stage 3: Implement Replay Engine
- Create replay test infrastructure:
  - Parse test log files to extract args, packets, version, receiver, config
  - Reconstruct `ConfigTarget` from parsed args
  - Feed packet sequences to configurator
  - Verify outgoing packet sequences match logged packets
  - Validate final configuration state matches logged config
- Manually create `replay_test.go` containing just a test function for each test case
- Be able to generate code `replay_test.go` automatically from `testdata/` directory

This approach provides comprehensive regression testing for the GPS configuration subsystem while maintaining clear traceability to real-world usage scenarios.