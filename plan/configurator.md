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

## Design

The new design replaces the current `NextRequest()/FindAck()/Done()` pattern with index-based request management. The key principle is that ConfigRequest is no longer exposed through the gpsprot interface - instead, all requests are addressed by their index.

### Core Design

```go
type RequestState int
const (
    StateNotReady RequestState = iota        // Dependencies not met
    StateReadyToSend                         // Can be sent now
    StateAwaitingResponse                    // Waiting for ACK/response
    StateSucceeded                           // Completed successfully  
    StateMayResend                           // Timed out, client can choose to retry
    StateFailed                              // Failed (NACK received or was sent but did not work)
    StateSkipped                             // Skipped (dependency failed, shouldn't be tried)
    StateEndOfRequests                       // No more requests in sequence
)

type Configurator interface {
    // Index-based request management
    GetPacket(index int) []byte              // Get packet (must be StateReadyToSend)
    GetSpeedChangeAfter(index int) int       // Get speed change (0 if none)
    SetSentTime(index int, tSent time.Time)  // Mark as sent
    GetState(index int) RequestState         // Current state
    
    // Timeout handling
    GetDeadline(index int) (time.Time, bool) // Only valid for StateAwaitingResponse
    SetTimedOut(index int)                   // StateAwaitingResponse → StateMayResend
    
    // Retry decision
    SetFailed(index int)                     // StateMayResend → StateFailed
    SetSkipped(index int)                    // StateNotReady → StateSkipped (dependency failed)
    // (SetSentTime again for StateMayResend → retry)
}
```

### Key Design Principles

1. **ConfigRequest Encapsulation**: The ConfigRequest type is no longer exposed through the gpsprot interface. Instead:
   - Requests are identified by their index (0, 1, 2, ...)
   - All request operations use the index as the handle
   - Request details remain internal to the protocol implementation
   - The caller cannot misuse or directly access ConfigRequest objects

2. **Automatic State Management**: The Configurator automatically handles state transitions based on received packets:
   - When an ACK is received, the request transitions to `StateSucceeded`
   - When a NACK is received, the request transitions to `StateFailed`
   - When a poll response is received, the request transitions to `StateSucceeded`
   - For certain errors, the Configurator may transition from `StateAwaitingResponse` directly to `StateFailed` (instead of allowing retries)
   - The caller no longer needs to call `FindAck()` and `Done()` - this happens internally

### Benefits

1. **Better Encapsulation**: ConfigRequest is no longer exposed, preventing misuse and simplifying the interface
2. **Simplified Caller Logic**: No need to call `FindAck()` and `Done()` - packet processing handles state transitions automatically
3. **Clear State Machine**: Explicit state transitions are easy to understand and test
4. **Sequential Processing**: `StateNotReady` ensures requests are processed in order
5. **Natural Retry Handling**: `StateMayResend` gives client explicit control over retry decisions
6. **Protocol Independence**: Index-based addressing works for any protocol implementation

### State Transitions

- `StateNotReady` → `StateReadyToSend` (when previous request succeeds)
- `StateReadyToSend` → `StateAwaitingResponse` | `StateSucceeded` (via SetSentTime)
- `StateAwaitingResponse` → `StateSucceeded` | `StateFailed` (via packet processing)
- `StateAwaitingResponse` → `StateMayResend` (via SetTimedOut)
- `StateAwaitingResponse` → `StateFailed` (via Configurator decision - no retry allowed)
- `StateMayResend` → `StateAwaitingResponse` (via SetSentTime retry)
- `StateMayResend` → `StateFailed` (via SetFailed)
- `StateNotReady` → `StateSkipped` (via SetSkipped when dependency fails)

### Implementation Notes

Implementations can use various internal structures to track requests. The UBX implementation uses a single slice of structs containing the request and its state, but this is an implementation detail. The key requirement is that the gpsprot interface only exposes indices, never ConfigRequest objects.

### Helper Orchestrator

To further simplify client usage, a helper struct manages the complexity:

```go
type ConfigAction int
const (
    ActionSendRequest ConfigAction = iota
    ActionWaitUntil
    ActionChangeSpeed  // After sending speed change request
    ActionError        // Unrecoverable error
    ActionSuccess      // Configuration complete
)

type ConfigInstruction struct {
    Action   ConfigAction
    Index    int           // For ActionSendRequest
    Packet   []byte       // For ActionSendRequest
    Speed    int          // For ActionChangeSpeed
    Deadline time.Time    // For ActionWaitUntil
    Error    error        // For ActionError
}

type ConfigOrchestrator struct {
    cfgtor      Configurator
    currentIdx  int
    retries     []int        // Track retries per request index, grows as needed
    maxRetries  int
}

func NewConfigOrchestrator(cfgtor Configurator, maxRetries int) *ConfigOrchestrator {
    return &ConfigOrchestrator{
        cfgtor:      cfgtor,
        retries:     nil,        // Start with nil, allocate only when needed
        maxRetries:  maxRetries,
    }
}

// Instructions returns an iterator over configuration instructions
// Using Go 1.23+ push-style iterators (range-over-func)
func (co *ConfigOrchestrator) Instructions() iter.Seq[ConfigInstruction] {
    return func(yield func(ConfigInstruction) bool) {
        for {
            state := co.cfgtor.GetState(co.currentIdx)
            
            switch state {
            case StateReadyToSend:
                packet := co.cfgtor.GetPacket(co.currentIdx)
                speed := co.cfgtor.GetSpeedChangeAfter(co.currentIdx)
                if !yield(ConfigInstruction{
                    Action: ActionSendRequest,
                    Index:  co.currentIdx,
                    Packet: packet,
                    Speed:  speed,
                }) {
                    return
                }
                
            case StateAwaitingResponse:
                deadline, _ := co.cfgtor.GetDeadline(co.currentIdx)
                if !yield(ConfigInstruction{
                    Action:   ActionWaitUntil,
                    Deadline: deadline,
                }) {
                    return
                }
                
            case StateMayResend:
                // Grow retries slice if needed
                for len(co.retries) <= co.currentIdx {
                    co.retries = append(co.retries, 0)
                }
                co.retries[co.currentIdx]++
                
                if co.retries[co.currentIdx] >= co.maxRetries {
                    co.cfgtor.SetFailed(co.currentIdx)
                }
                // Loop continues to get next state
                
            case StateSucceeded:
                co.currentIdx++
                // Loop continues to get next request
                
            case StateFailed, StateSkipped:
                yield(ConfigInstruction{
                    Action: ActionError,
                    Error:  fmt.Errorf("configuration failed at request %d", co.currentIdx),
                })
                return
                
            case StateEndOfRequests:
                yield(ConfigInstruction{Action: ActionSuccess})
                return
            }
        }
    }
}
```

### Simple Client Usage

```go
orchestrator := gpsprot.NewConfigOrchestrator(configurator, maxRetries)

for instruction := range orchestrator.Instructions() {
    switch instruction.Action {
    case gpsprot.ActionSendRequest:
        if instruction.Speed != 0 {
            // Serial port specific - send then change speed
            serPort.WriteThenChangeSpeed(instruction.Packet, instruction.Speed)
        } else {
            port.Write(instruction.Packet)
        }
        cfgtor.SetSentTime(instruction.Index, time.Now())
        
    case gpsprot.ActionWaitUntil:
        select {
        case <-time.After(time.Until(instruction.Deadline)):
            // Iterator will handle the timeout on next iteration
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

### Sequential Processing

The design enforces strict sequential processing as required by UBX protocol:

**Sequential Enforcement**: The `nextIndex` field tracks the first request that hasn't succeeded yet. Only this request can be `StateReadyToSend`.

**Simple State Logic**:
- Requests before `nextIndex`: Already processed (show their actual state)
- Request at `nextIndex`: Current request being processed
- Requests after `nextIndex`: Always `StateNotReady`

**Abort on Failure**: When a request fails (NACK or too many retries), configuration stops and the Configurator initiates recovery operations.

### Replay Testing Compatibility

The index-based design enhances replay testing:

**Preserved Determinism**: The Configurator remains completely deterministic. All timing information comes from recorded timestamps, and packet processing follows the same logic as the current system.

**Enhanced Test Clarity**: Instead of tracking complex FindAck/Done sequences, replay tests verify straightforward state transitions. Test assertions become clearer: "after feeding packet X, request 5 should be StateSucceeded."

**Better Error Reproduction**: Failed scenarios are easier to reproduce because request states are explicit. Tests can verify exactly which requests succeeded, failed, or were skipped.

**Simplified Test Logic**: The replay harness becomes simpler because it no longer needs to simulate the caller's FindAck/Done logic - it just feeds packets and checks resulting states against expected values.

This design addresses all the main weaknesses while preserving testability and making protocol implementations straightforward to write and maintain.

### UBX Implementation Outline

```go
// internal/ubx/ubxcfg.go

// requestState tracks the state of a single configuration request (unexported)
type requestState struct {
    request   gpsprot.ConfigRequest  // The actual request (msgRequest, pollRequest, etc.)
    sentTime  time.Time              // When the request was sent
    state     gpsprot.RequestState   // Current state
}

type Configurator struct {
    // New index-based tracking
    requests  []requestState         // All requests and their state
    nextIndex int                    // Index of first request that hasn't succeeded
    
    // Existing fields remain
    ver       *Version
    acks      ackList
    tRead     map[bin.MsgID]time.Time
    raw       RawConfig
    origPrt   *bin.CfgPrt
    monGNSS   *monGNSS
    steps     []func(*Configurator) error
    stepIndex int
    target    *gpsprot.ConfigTarget
    survey    bool
}

// Implementation of new interface methods
func (c *Configurator) GetState(index int) gpsprot.RequestState {
    // Generate more requests if needed
    for index >= len(c.requests) && c.stepIndex < len(c.steps) {
        err := c.steps[c.stepIndex](c)
        c.stepIndex++
        if err != nil {
            return gpsprot.StateEndOfRequests
        }
    }
    
    if index >= len(c.requests) {
        return gpsprot.StateEndOfRequests
    }
    
    if index < c.nextIndex {
        return c.requests[index].state  // Already processed
    } else if index > c.nextIndex {
        return gpsprot.StateNotReady     // Future request
    }
    
    // This is the current request (index == c.nextIndex)
    rs := &c.requests[index]
    
    // Update nextIndex if this request succeeded
    if rs.state == gpsprot.StateSucceeded {
        c.nextIndex++
        // Set next request to ready if it exists
        if c.nextIndex < len(c.requests) {
            c.requests[c.nextIndex].state = gpsprot.StateReadyToSend
        }
    }
    
    return rs.state
}

func (c *Configurator) GetPacket(index int) []byte {
    if index < len(c.requests) {
        return c.requests[index].request.Packet()
    }
    return nil
}

func (c *Configurator) GetSpeedChangeAfter(index int) int {
    if index < len(c.requests) {
        return c.requests[index].request.ChangeSpeed()
    }
    return 0
}

func (c *Configurator) SetSentTime(index int, tSent time.Time) {
    if index < len(c.requests) {
        rs := &c.requests[index]
        rs.sentTime = tSent
        if rs.request.Ackable() || rs.request.AwaitingResponse(tSent) {
            rs.state = gpsprot.StateAwaitingResponse
        } else {
            rs.state = gpsprot.StateSucceeded
        }
    }
}

func (c *Configurator) GetDeadline(index int) (time.Time, bool) {
    if index < len(c.requests) && c.requests[index].state == gpsprot.StateAwaitingResponse {
        req := c.requests[index].request
        // Deadline is determined by the request type and protocol rules
        if req.Ackable() || req.AwaitingResponse(c.requests[index].sentTime) {
            return c.requests[index].sentTime.Add(1500 * time.Millisecond), true
        }
    }
    return time.Time{}, false
}

func (c *Configurator) SetTimedOut(index int) {
    if index < len(c.requests) {
        c.requests[index].state = gpsprot.StateMayResend
    }
}

func (c *Configurator) SetFailed(index int) {
    if index < len(c.requests) {
        c.requests[index].state = gpsprot.StateFailed
        c.stop() // Trigger recovery
    }
}

func (c *Configurator) SetSkipped(index int) {
    if index < len(c.requests) {
        c.requests[index].state = gpsprot.StateSkipped
        // Don't trigger recovery - this is expected when dependencies fail
    }
}

// Modified addRequest to work with new structure
func (c *Configurator) addRequest(req gpsprot.ConfigRequest) error {
    state := gpsprot.StateNotReady
    // First request starts as ready
    if len(c.requests) == 0 {
        state = gpsprot.StateReadyToSend
    }
    
    c.requests = append(c.requests, requestState{
        request: req,
        state:   state,
    })
    return nil
}

// Process incoming packets - update request status based on ACKs
func (c *Configurator) processMsg(msg bin.Msg, t time.Time) (bool, error) {
    switch mt := msg.(type) {
    case *bin.AckAck:
        // Update ack list as before
        c.acks.ack(mt.MsgID, true, t)
        
        // Check if current request is waiting for this ACK
        if c.nextIndex < len(c.requests) && 
           c.requests[c.nextIndex].state == gpsprot.StateAwaitingResponse {
            req := c.requests[c.nextIndex].request
            if req.Ackable() && c.findAckForRequest(req, c.requests[c.nextIndex].sentTime) != nil {
                c.requests[c.nextIndex].state = gpsprot.StateSucceeded
                req.Done() // Update raw config
            }
        }
        return true, nil
        
    case *bin.AckNak:
        c.acks.ack(mt.MsgID, false, t)
        
        if c.nextIndex < len(c.requests) && 
           c.requests[c.nextIndex].state == gpsprot.StateAwaitingResponse {
            req := c.requests[c.nextIndex].request
            if req.Ackable() && c.findAckForRequest(req, c.requests[c.nextIndex].sentTime) != nil {
                c.requests[c.nextIndex].state = gpsprot.StateFailed
                c.stop() // Trigger recovery
            }
        }
        return true, nil
        
    case *bin.MonGnss:
        c.tRead[mt.ID()] = t
        c.monGNSS = c.newMonGNSS(mt)
        // Check if this satisfies a poll response
        if c.nextIndex < len(c.requests) && 
           c.requests[c.nextIndex].state == gpsprot.StateAwaitingResponse {
            req := c.requests[c.nextIndex].request
            if !req.AwaitingResponse(c.requests[c.nextIndex].sentTime) {
                c.requests[c.nextIndex].state = gpsprot.StateSucceeded
            }
        }
        return true, nil
    }
    
    // Handle other message types as before
    mid := msg.ID()
    if mid.CfgClass() {
        c.tRead[mid] = t
        _, err := c.raw.AddMsg(msg)
        // Check if this satisfies a poll response
        if c.nextIndex < len(c.requests) && 
           c.requests[c.nextIndex].state == gpsprot.StateAwaitingResponse {
            req := c.requests[c.nextIndex].request
            if !req.AwaitingResponse(c.requests[c.nextIndex].sentTime) {
                c.requests[c.nextIndex].state = gpsprot.StateSucceeded
            }
        }
        return err == nil, err
    }
    
    return false, nil
}

func (c *Configurator) findAckForRequest(req gpsprot.ConfigRequest, tSent time.Time) *gpsprot.Ack {
    packet := req.Packet()
    msgID := bin.PacketMsgId(packet)
    a := c.acks.findAckByMsgId(msgID, tSent)
    if a == nil {
        return nil
    }
    return &a.Ack
}
```