# GPS Protocol Configurator Interface Redesign

## Overview

This document analyzes the current Configurator interface in `internal/gpsprot/` and proposes improvements to address design limitations while preserving the key property of testability.

## How the Current Design Works

### Key Components

1. **PacketExchanger** - Protocol-specific factory that creates a Configurator
   - Handles the probing phase to detect if receiver is responding
   - Creates a Configurator for the actual configuration phase

2. **Configurator** - Manages configuration state machine for a specific protocol
   - Generates configuration requests via `NextRequest()`
   - Processes received packets to track acknowledgments
   - Updates internal state based on responses
   - No I/O, no timing - purely deterministic state management

3. **ConfigRequest** - Represents a single configuration command
   - Provides the packet bytes to send
   - Indicates if speed change is needed after sending
   - Specifies pause duration after acknowledgment
   - Indicates if acknowledgment is expected
   - Has `Done()` method to update Configurator state when successful

4. **Caller (gpscfg)** - Orchestrates the configuration process
   - Manages I/O with the GPS receiver
   - Handles timing and retries
   - Calls `FindAck()` to check for acknowledgments
   - Calls `Done()` when request succeeds

### Configuration Flow

1. **Detection Phase**:
   - Caller reads packets from GPS to verify it's working
   - Validates checksums to ensure clean communication

2. **Probe Phase**:
   - PacketExchanger provides probe packet
   - Caller sends it and waits for response
   - PacketExchanger's `ProbeOK()` indicates success

3. **Configuration Phase**:
   - Configurator generates requests via `NextRequest()`
   - For each request:
     - Caller sends the packet
     - If acknowledgment expected, caller waits and checks via `FindAck()`
     - If successful, caller calls `Done()` on the request
     - If failed, caller may retry or call `Abort()`
   - Process continues until `NextRequest()` returns nil

### Testability Design

The current design achieves testability by:
- Configurator has no I/O dependencies
- All timing decisions are made by the caller
- Packet processing is deterministic
- State transitions are predictable

The replay test system can feed recorded packets to the Configurator and verify it generates the expected configuration sequence.

## Current Design Strengths

### 1. Excellent Testability
- Configurator is completely deterministic with no I/O or timing dependencies
- Replay test system can thoroughly validate configuration sequences
- Clear separation between protocol logic and I/O/timing concerns

### 2. Clean Separation of Concerns
- Protocol-specific logic isolated in Configurator
- I/O and timing handled by caller
- Each component has clear responsibilities

### 3. Protocol Abstraction
- Common interface allows multiple protocol implementations
- Caller (gpscfg) can work with any protocol that implements the interface

### 4. State Management
- Configurator maintains configuration state cleanly
- Can track what has been configured and what remains

## Current Design Weaknesses

### 1. Complex Caller Responsibilities
The caller must:
- Call `FindAck()` at the right time to check for acknowledgments
- Call `Done()` when acknowledgment is positive
- Handle the distinction between acknowledgments and poll responses

This creates opportunities for errors and makes the interface harder to use correctly.

### 2. Protocol-Specific Concepts Leak Through
- The distinction between "acks" and "responses" is protocol-specific
- Not all protocols may have this same model
- The `AwaitingResponse()` method exposes internal protocol timing

### 3. No Batching Support
Current design assumes synchronous request/response pattern. Some operations could be batched together and sent simultaneously, but the API doesn't accommodate this.

### 4. Protocol-Specific Timing in Caller
The caller (gpscfg) makes timing decisions that should be protocol-specific:
- Hardcoded 1500ms timeout for acknowledgments
- Speed change delay assumptions (400ms)
- These values may not be appropriate for all protocols

### 5. Implementation Complexity
For new protocol implementations:
- Must carefully manage acknowledgment tracking
- Need to implement FindAck with proper timing considerations
- Easy to get wrong, especially with edge cases

### 6. No Dependency Management
Current design has no concept of request dependencies:
- Common pattern: GET current value → SET new value (if GET fails, SET should be skipped)
- Speed changes require all previous requests to complete first
- GNSS resets invalidate subsequent requests until reset completes
- Currently any NACK causes complete abort, even if other independent operations could continue

### 7. Limited Error Recovery
- Any configuration failure stops the entire process
- Cannot skip failed optional operations while continuing with required ones
- No selective recovery based on which operations actually failed

## Open Design Questions

### Retry Awareness
- Should the Configurator be aware when requests are retried?
- Would this help with protocol-specific recovery strategies?
- How would this impact testability and replay?
- Currently retries are not logged, creating challenges for replay testing

## Design Options

### A. Index-Based Request Management with Parallel Slices

This approach replaces the current `NextRequest()/FindAck()/Done()` pattern with index-based request lifecycle management.

#### Core Design

```go
type RequestStatus int
const (
    StatusNotExists RequestStatus = iota     // Index beyond current requests
    StatusNotReady                           // Dependencies not met
    StatusReadyToSend                        // Can be sent now
    StatusSent                               // Just sent, transitioning...
    StatusAwaitingResponse                   // Waiting for ACK/response
    StatusSucceeded                          // Completed successfully  
    StatusRejected                           // Received NACK
    StatusMayResend                          // Timed out, client can choose to retry
    StatusSkipped                            // Skipped (dependency failed or client gave up)
    StatusEndOfRequests                      // No more requests in sequence
)

type Configurator interface {
    // Index-based request management
    RequestPacket(index int) []byte          // Get packet (must be ReadyToSend)
    RequestSent(index int, tSent time.Time)  // Mark as sent
    RequestStatus(index int) RequestStatus   // Current status
    
    // Timeout handling
    GetDeadline(index int) (time.Time, bool) // Only valid for StatusAwaitingResponse
    SetTimedOut(index int)                   // StatusAwaitingResponse → StatusMayResend
    
    // Retry decision
    GiveUp(index int)                        // StatusMayResend → StatusSkipped
    // (RequestSent again for StatusMayResend → retry)
    
    // Discovery
    ReadyRequests() []int                    // Indices ready to send
}
```

#### Parallel Slice Architecture

The implementation uses separate slices to track requests and their status:

```go
type Configurator struct {
    requests []ConfigRequest    // Permanent slice of all requests
    statuses []RequestStatus    // Parallel slice tracking each request's status
    sentTimes []time.Time       // When each request was sent
    attempts []int              // Retry count for each request
    // ... other existing fields
}
```

#### Benefits

1. **ConfigRequest Stays Private**: No longer exposed to caller, preventing misuse
2. **Clear State Machine**: Explicit status transitions are easy to understand and test
3. **Dynamic Discovery**: Client discovers requests incrementally as they're generated
4. **Dependency Support**: `StatusNotReady` prevents sending requests before dependencies complete
5. **Selective Recovery**: Failed requests can be skipped while independent ones continue
6. **Natural Retry Handling**: `StatusMayResend` gives client explicit control over retry decisions
7. **Batching Support**: Multiple requests can be marked `ReadyToSend` simultaneously for batch operations

#### Status Transitions

- `StatusNotReady` → `StatusReadyToSend` (when dependencies complete)
- `StatusReadyToSend` → `StatusSent` → `StatusAwaitingResponse` (via RequestSent)
- `StatusAwaitingResponse` → `StatusSucceeded` | `StatusRejected` (via packet processing)
- `StatusAwaitingResponse` → `StatusMayResend` (via SetTimedOut)
- `StatusMayResend` → `StatusAwaitingResponse` (via RequestSent retry)
- `StatusMayResend` → `StatusSkipped` (via GiveUp)

#### UBX Implementation Adaptation

The existing UBX code can be adapted with minimal changes:

1. **Keep Existing Request Types**: All current `msgRequest`, `pollRequest`, etc. can be reused
2. **Change from Queue to Permanent Slice**: Instead of popping from `c.reqs`, keep all requests
3. **Add Parallel Status Tracking**: Add `statuses []RequestStatus` alongside the requests slice
4. **Update Packet Processing**: Modify `processMsg()` to update request status instead of just ACK tracking
5. **Preserve Domain Logic**: All the existing step functions and configuration logic remain unchanged

Example adaptation:
```go
// Current: c.reqs = append(c.reqs, req)
// New: 
c.requests = append(c.requests, req)
c.statuses = append(c.statuses, StatusReadyToSend)
c.sentTimes = append(c.sentTimes, time.Time{})
c.attempts = append(c.attempts, 0)
```

#### Helper Orchestrator

To further simplify client usage, a helper struct manages the complexity:

```go
type ConfigAction int
const (
    ActionSendRequest ConfigAction = iota
    ActionWaitUntil
    ActionError        // Unrecoverable error
    ActionSuccess      // Configuration complete
)

type ConfigInstruction struct {
    Action   ConfigAction
    Index    int           // For ActionSendRequest
    Deadline time.Time     // For ActionWaitUntil
    Error    error         // For ActionError
}

type ConfigOrchestrator struct {
    cfgtor    Configurator
    maxIndex  int
    retryPolicy RetryPolicy
}

func (co *ConfigOrchestrator) NextInstruction() ConfigInstruction {
    // Handles request discovery, timeout management, retry decisions
    // Returns simple instructions for the client to execute
}
```

#### Simple Client Usage

```go
orchestrator := gpsprot.NewConfigOrchestrator(configurator, retryPolicy)

for {
    instruction := orchestrator.NextInstruction()
    
    switch instruction.Action {
    case gpsprot.ActionSendRequest:
        packet := cfgtor.RequestPacket(instruction.Index)
        send(packet)
        cfgtor.RequestSent(instruction.Index, time.Now())
        
    case gpsprot.ActionWaitUntil:
        select {
        case <-time.After(time.Until(instruction.Deadline)):
            // NextInstruction() will handle the timeout
        case packet := <-packetCh:
            // Packet processing happens via NativeMsgHandler
        }
        
    case gpsprot.ActionError:
        return instruction.Error
        
    case gpsprot.ActionSuccess:
        return nil // Configuration complete
    }
}
```

#### Dependency Management

The index-based design naturally supports request dependencies:

**Dependency Tracking**: Each request can specify which other requests it depends on. The Configurator tracks these relationships and only marks requests as `StatusReadyToSend` when their dependencies have `StatusSucceeded`.

**Common Dependency Patterns**:
- **GET→SET Operations**: Request 5 polls current GNSS config, Request 6 sets new config (depends on 5 succeeding)
- **Sequential Operations**: Speed change requires all prior requests to complete first
- **Recovery Chains**: If Request 5 fails, Request 6 becomes `StatusSkipped` but Request 7 (independent) can still proceed

**Selective Recovery**: When a request fails, only its dependents are marked `StatusSkipped`. Independent operations continue normally, achieving partial configuration success rather than complete failure.

#### Replay Testing Compatibility

The index-based design enhances replay testing:

**Preserved Determinism**: The Configurator remains completely deterministic. All timing information comes from recorded timestamps, and packet processing follows the same logic as the current system.

**Enhanced Test Clarity**: Instead of tracking complex FindAck/Done sequences, replay tests verify straightforward status transitions. Test assertions become clearer: "after feeding packet X, request 5 should be StatusSucceeded."

**Better Error Reproduction**: Failed scenarios are easier to reproduce because request states are explicit. Tests can verify exactly which requests succeeded, failed, or were skipped.

**Simplified Test Logic**: The replay harness becomes simpler because it no longer needs to simulate the caller's FindAck/Done logic - it just feeds packets and checks resulting statuses against expected values.

This design addresses all the main weaknesses while preserving testability and making protocol implementations straightforward to write and maintain.