# Configurator2 Migration Plan

## Overview

This document outlines the complete migration plan from the existing ConfigProtocol/Configurator interfaces to the new ConfigProtocol2/Configurator2/ConfigDirector design. The migration involves multiple branches and careful sequencing to maintain functionality while transitioning.

## Current State

### Branches
- **master**: Has original ConfigProtocol/Configurator interfaces, UBX works with old design
- **configurator-redesign-136**: Has config2.go with new interfaces (ConfigProtocol2, Configurator2, ConfigDirector)
- **unicore**: Has UNC implementation of ConfigProtocol2, tested with ConfigDirector

### Issues
- gpscfg uses old ConfigProtocol interface
- Replay tests are tightly coupled to UBX protocol
- Cannot test Unicore configuration with replay testing
- UBX doesn't implement ConfigProtocol2 yet

## Migration Phases

### Phase 0: Refactor Replay Tests on Master (Prerequisite)

**Goal**: Separate protocol-specific logic from replay test framework to enable multi-protocol testing.

#### Steps

1. **Create protocol-specific test structure**
   ```
   git checkout master
   ```
   - Rename `replay_files_test.go` → `replay_ubx_test.go`

2. **Extract protocol-specific logic**
   - Create packet comparison function type in `replay_test.go`:
     ```go
     type packetCompareFn func(t *testing.T, msgID string, actual []byte, expected gpsio.PacketLogEntry) bool
     ```
   - Move UBX-specific code to `replay_ubx_test.go`:
     - `TestReplay()` function → rename to `TestReplayUBX()`
     - `replayFiles` variable (list of UBX test files)
     - `valsetPacketsEqual()` function
     - `valgetPacketsEqual()` function
     - UBX packet parsing from `packetsEqual()`
     - `ubxSchema` variable
   - Create `ubxPacketsEqual` function that handles UBX-specific comparisons

3. **Update replay framework in `replay_test.go`**
   - Keep `testReplayFile()` as the core replay engine (make it accept comparison function)
   - Update function signature: `testReplayFile(t *testing.T, name string, comparePackets packetCompareFn)`
   - Add `comparePackets packetCompareFn` field to `replayer` struct
   - Update `newReplayer()` to accept comparison function parameter
   - Update `packetsEqual()` to delegate to the comparison function
   - Remove UBX-specific imports (move to replay_ubx_test.go)

4. **Test and commit**
   ```bash
   go test ./internal/gpscmd
   git add -A
   git commit -m "Refactor replay tests for protocol independence

   - Extract UBX-specific packet comparison to replay_ubx_test.go
   - Use function type for protocol-specific packet comparison
   - Prepare for testing multiple GPS protocols
   - No functional changes, just reorganization"
   git push origin master
   ```

### Phase 1: Update Infrastructure on configurator-redesign-136

**Goal**: Switch gpscfg and gpsreg to use ConfigProtocol2 and ConfigDirector.

#### Steps

1. **Merge master changes and switch branch**
   ```bash
   git checkout configurator-redesign-136
   git merge master  # Get replay test refactoring
   ```

2. **Update gpsreg** (`internal/gpsreg/reg.go`)
   - Change `CreateConfigProtocols()` to return `[]gpsprot.ConfigProtocol2`
   - Initially return empty list (no protocols available yet)

3. **Update gpscfg** (`internal/gpscfg/gpscfg.go`)
   - Replace `configure()` method signature to accept `ConfigProtocol2`
   - Implement using ConfigDirector based on configurator.md example:
     ```go
     func (mh *msgHandler) configure(ctx context.Context, prot gpsprot.ConfigProtocol2, target *gpsprot.ConfigTarget, port gpsio.OutPort) (*gpsprot.ConfigProps, *gpsprot.ReceiverInfo, error) {
         cfgtor, err := prot.Configure2(target)
         if err != nil {
             return nil, nil, err
         }
         
         director := gpsprot.NewConfigDirector(cfgtor, maxRetries)
         
         for action := range director.Actions() {
             switch action.Type {
             case gpsprot.ConfigActionSendRequest:
                 // Handle send
             case gpsprot.ConfigActionCheckTimeout:
                 // Handle timeout check
             case gpsprot.ConfigActionWaitUntil:
                 // Handle wait
             case gpsprot.ConfigActionError:
                 // Handle error
             }
         }
         
         return cfgtor.ConfigProps(), cfgtor.ReceiverInfo(), director.ErrorCount == 0 ? nil : knownErr
     }
     ```

4. **Update gpscmd** (`internal/gpscmd/gpscmd.go`)
   - Update to use `ConfigProtocol2`
   - Adjust probe and configuration calls

5. **Update replay tests to use ConfigDirector** (`internal/gpscmd/replay_test.go`)
   - Port `replayer.run()` to use ConfigProtocol2 and ConfigDirector
   - Update based on configurator.md example:
     - Replace `NextRequest()/FindAck()/Done()` pattern with ConfigDirector
     - Handle ConfigAction types (Send, CheckTimeout, WaitUntil, Error)
   - Temporarily disable UBX tests in `replay_ubx_test.go`:
     - Rename `TestReplayUBX` → `DisabledTestReplayUBX` (or use `t.Skip()`)
     - Tests will be re-enabled after UBX is ported in Phase 3

6. **Update daemon if needed** (`internal/daemon/daemon.go`)
   - Check if it directly uses ConfigProtocol
   - Update to ConfigProtocol2 if necessary

7. **Commit and push**
   ```bash
   git add -A
   git commit -m "Update infrastructure to use ConfigProtocol2 and ConfigDirector

   - Switch gpsreg to ConfigProtocol2 interface
   - Update gpscfg to use ConfigDirector for orchestration
   - Update gpscmd to use new interface
   - Port replay tests to use ConfigDirector
   - Temporarily disable UBX replay tests (no ConfigProtocol2 yet)"
   git push origin configurator-redesign-136
   ```

### Phase 2: Test with Unicore Hardware

**Goal**: Validate the new design with real Unicore hardware.

#### Steps

1. **Merge to unicore branch**
   ```bash
   git checkout unicore
   git merge configurator-redesign-136
   ```

2. **Register UNC in gpsreg** (`internal/gpsreg/reg.go`)
   ```go
   func CreateConfigProtocols2() []gpsprot.ConfigProtocol2 {
       return []gpsprot.ConfigProtocol2{
           unc.NewConfigProtocol(),
       }
   }
   ```

3. **Add Unicore replay tests** (if test data available)
   - Create `replay_unc_test.go` with:
     - `TestReplayUNC()` function
     - `uncReplayFiles` variable (list of UNC test files)
     - `uncPacketsEqual` function for UNC-specific comparisons
   - Add `.jsonl` test files to `testdata/`

4. **Test with hardware**
   - Run `satpulsetool gps` with Unicore receiver
   - Test various configuration operations
   - Verify satpulsed can configure Unicore receivers

5. **Fix issues and commit**
   ```bash
   git add -A
   git commit -m "Enable UNC in gpsreg for ConfigProtocol2

   - Register Unicore ConfigProtocol2 implementation
   - Add Unicore replay tests (if applicable)
   - Tested with UM980 hardware"
   git push origin unicore
   ```

### Phase 3: Port UBX to ConfigProtocol2

**Goal**: Update UBX to implement the new interfaces.

#### Steps

1. **Switch back to configurator-redesign-136**
   ```bash
   git checkout configurator-redesign-136
   ```

2. **Port UBX implementation**
   
   **File: `internal/ubx/ubxcfgprot.go`**
   - Implement `Configure2()` method:
     ```go
     func (cp *ConfigProtocol) Configure2(target *gpsprot.ConfigTarget) (gpsprot.Configurator2, error) {
         // Create and return UBX Configurator2 implementation
     }
     ```

   **File: `internal/ubx/ubxcfg.go`**
   - Update `Configurator` to implement `Configurator2` interface
   - Implement required methods:
     - `GenerateRequests() error`
     - `GetRequestCount() (int, bool)`
     - `Request(int) ConfigRequest2`
   - Update request types to implement `ConfigRequest2`

3. **Register UBX in gpsreg**
   ```go
   func CreateConfigProtocols2() []gpsprot.ConfigProtocol2 {
       return []gpsprot.ConfigProtocol2{
           ubx.NewConfigProtocol(),
       }
   }
   ```

4. **Re-enable UBX replay tests**
   - Rename `DisabledTestReplayUBX` → `TestReplayUBX` in `replay_ubx_test.go`
   - Tests should now work with UBX's ConfigProtocol2 implementation

5. **Test thoroughly**
   ```bash
   go test ./internal/ubx
   go test ./internal/gpscmd  # Replay tests
   ```

6. **Test with hardware if available**
   - Test with u-blox receivers
   - Verify all configuration operations work

7. **Commit**
   ```bash
   git add -A
   git commit -m "Port UBX to ConfigProtocol2 interface

   - Implement Configure2() for UBX
   - Port UBX Configurator to Configurator2
   - Update replay tests to use ConfigDirector
   - All tests passing"
   git push origin configurator-redesign-136
   ```

### Phase 4: Cleanup

**Goal**: Remove old interfaces now that both UBX and UNC use the new ones.

#### Steps

1. **Remove old interfaces from `gpsprot`** (still on configurator-redesign-136)
   - Delete old interface definitions from `config.go`:
     - `ConfigProtocol` interface
     - `Configurator` interface  
     - `ConfigRequest` interface
     - Related types (Ack, etc.) if no longer used

2. **Rename new interfaces** (optional but recommended):
   - `ConfigProtocol2` → `ConfigProtocol`
   - `Configurator2` → `Configurator`
   - `ConfigRequest2` → `ConfigRequest`
   - Update all references throughout the codebase

3. **Test everything still works**
   ```bash
   go test ./...
   ```

4. **Commit cleanup**
   ```bash
   git add -A
   git commit -m "Remove old Configurator interfaces

   - Delete ConfigProtocol/Configurator/ConfigRequest
   - Rename ConfigProtocol2 → ConfigProtocol (optional)
   - All protocols now use new interface"
   git push origin configurator-redesign-136
   ```

### Phase 5: Final Merges

**Goal**: Integrate all changes to master and unicore.

#### Steps

1. **Merge to master**
   ```bash
   git checkout master
   git merge --no-squash configurator-redesign-136
   git push origin master
   ```
   - Now master has complete new design with both UBX and cleanup

2. **Merge master to unicore**
   ```bash
   git checkout unicore
   git merge master
   ```
   - Resolve any conflicts (both UBX and UNC in gpsreg)
   - Update gpsreg to include both protocols:
     ```go
     func CreateConfigProtocols() []gpsprot.ConfigProtocol {
         return []gpsprot.ConfigProtocol{
             ubx.NewConfigProtocol(),
             unc.NewConfigProtocol(),
         }
     }
     ```

3. **Final testing and push**
   ```bash
   go test ./...
   git push origin unicore
   ```

## Timeline and Dependencies

```
Phase 0 (master)
    ↓
Phase 1 (configurator-redesign-136)
    ↓
Phase 2 (unicore) ← Test with hardware
    ↓
Phase 3 (configurator-redesign-136)
    ↓
Phase 4 (configurator-redesign-136) ← Cleanup
    ↓
Phase 5 (master, then unicore) ← Final merges
```

## Risk Mitigation

1. **Phase 0**: Low risk - just refactoring tests, no functional changes
2. **Phase 1**: Medium risk - no protocols work temporarily (acceptable on feature branch)
3. **Phase 2**: Low risk - only affects unicore branch
4. **Phase 3**: Medium risk - UBX migration complexity
5. **Phase 4**: Low risk - cleanup of unused code
6. **Phase 5**: Low risk - standard merge operations

## Success Criteria

- [ ] Replay tests work for both UBX and UNC protocols
- [ ] gpscfg uses ConfigDirector for all configuration
- [ ] Both UBX and UNC implement ConfigProtocol2
- [ ] All existing tests pass
- [ ] Hardware testing successful for both protocols
- [ ] Clean git history with --no-squash merges

## Benefits After Migration

1. **Cleaner architecture**: ConfigDirector handles all orchestration complexity
2. **Better testability**: Same code paths for production and tests
3. **Multi-protocol support**: Easy to add new protocols
4. **Improved error handling**: ConfigDirector manages retries consistently
5. **Simplified client code**: gpscfg is much simpler with ConfigAction pattern