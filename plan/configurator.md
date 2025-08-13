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

The new design replaces the current `NextRequest()/FindAck()/Done()` pattern with a clean request-based interface. Instead of index-based method calls, clients get a `ConfigRequest` handle that encapsulates both the request data and its execution state.

### Core Design

```go
type ConfigRequestState int
const (
    ConfigRequestNotReady ConfigRequestState = iota
    ConfigRequestReadyToSend
    ConfigRequestAwaitingResponse
    ConfigRequestPausing
    ConfigRequestSucceeded
    ConfigRequestMayResend
    ConfigRequestFailed
    ConfigRequestSkipped
)

type ConfigRequest interface {
    GetPacket() []byte
    GetSpeedChangeAfter() int
    GetState() ConfigRequestState
    GetResponseDeadline() time.Time  // valid when state is ConfigRequestAwaitingResponse
    GetReadyTime() time.Time         // valid when state is ConfigRequestPausing
    GetError() error                 // valid when state is ConfigRequestFailed

    SetSentTime(tSent time.Time)
    SetTimedOut()
    SetWontResend()
}

type Configurator interface {
    GenerateRequests() error
    GetRequestCount() (int, bool)
    Request(index int) ConfigRequest
}
```

The state of the Configurator consists of a slice of requests and state that can generate additional requests. Each request has a state.

#### ConfigRequestState Semantics

**ConfigRequestNotReady**: This request cannot be sent yet because some earlier requests are not yet in the Succeeded state.

**ConfigRequestReadyToSend**: This request is ready to be sent to the GPS receiver.

**ConfigRequestAwaitingResponse**: The request has been sent and the Configurator is expecting to receive one or more packets in response from the receiver.

**ConfigRequestPausing**: The request received its acknowledgment/response and is waiting for a protocol-specified pause duration before the next request can be sent.

**ConfigRequestSucceeded**: The request completed successfully.

**ConfigRequestMayResend**: The request timed out waiting for a response and is eligible for retry. The client must decide whether to retry (via SetSentTime) or abandon (via SetWontResend).

**ConfigRequestFailed**: The request was sent but did not succeed and cannot be retried.

**ConfigRequestSkipped**: This request was skipped because an earlier request did not succeed.

#### State Transitions  

ConfigRequestState values change in response to:
- **Set* method calls** by the client
- **Packet processing** when the Configurator receives GPS packets

**Client-initiated transitions:**
- `SetSentTime()`: ConfigRequestReadyToSend → ConfigRequestAwaitingResponse (if response expected) or ConfigRequestSucceeded (if no response expected)
- `SetSentTime()`: ConfigRequestMayResend → ConfigRequestAwaitingResponse (client chooses to retry)
- `SetTimedOut()`: ConfigRequestAwaitingResponse → ConfigRequestMayResend
- `SetWontResend()`: ConfigRequestMayResend → ConfigRequestFailed

**Automatic transitions from packet processing:**
- Positive ACK received + pause needed: ConfigRequestAwaitingResponse → ConfigRequestPausing
- Positive ACK received + no pause: ConfigRequestAwaitingResponse → ConfigRequestSucceeded
- NACK received: ConfigRequestAwaitingResponse → ConfigRequestFailed  
- Expected poll response received + pause needed: ConfigRequestAwaitingResponse → ConfigRequestPausing
- Expected poll response received + no pause: ConfigRequestAwaitingResponse → ConfigRequestSucceeded
- Protocol error detected: ConfigRequestAwaitingResponse → ConfigRequestFailed
- Pause duration elapsed: ConfigRequestPausing → ConfigRequestSucceeded

**Configurator-managed transitions:**
- When a request reaches ConfigRequestSucceeded, the next ConfigRequestNotReady request automatically becomes ConfigRequestReadyToSend
- ConfigRequestNotReady → ConfigRequestSkipped (when the Configurator determines a dependency has failed)

#### Configurator Interface Methods

All precondition failures result in panics. All Get* methods are side-effect free and do not modify the Configurator state.

**GenerateRequests() error**:
- Attempts to generate more requests, potentially increasing the slice size returned by GetRequestCount()
- This is the only method that can increase the slice size
- Generation is lazy - it may generate only some requests if later requests depend on the results of earlier ones
- Should be called when the client needs more requests than are currently available
- Returns an error if request generation fails

**GetRequestCount() (int, bool)**:
- Returns the current number of requests in the slice and whether the slice is complete
- When bool is true, the slice count will never increase
- When bool is false, the slice count may increase after calling GenerateRequests()

**GetState(index int) ConfigRequestState**:
- Precondition: `index < GetRequestCount()`
- Returns the current state of the request at the given index
- This is the primary method for tracking configuration progress

**GetPacket(index int) []byte**:
- Precondition: `index < GetRequestCount()`
- Precondition: request state is ConfigRequestReadyToSend, ConfigRequestMayResend, or ConfigRequestFailed
- Returns the packet bytes for the request at the given index
- The returned packet is ready to transmit to the GPS receiver without modification
- For ConfigRequestMayResend and ConfigRequestFailed states, this is useful for error reporting purposes

**GetSpeedChangeAfter(index int) int**:
- Precondition: `index < GetRequestCount()`
- Precondition: same states as GetPacket
- Returns the new baud rate to configure after sending this request
- Returns 0 if no speed change is required
- Speed changes are protocol-specific operations that must be handled by the client after the packet is sent but before waiting for acknowledgment

**GetResponseDeadline(index int) time.Time**:
- Precondition: `index < GetRequestCount()`
- Precondition: request state is ConfigRequestAwaitingResponse
- Returns the absolute time by which all response packets must have been received
- The returned time is non-zero and includes monotonic time for accurate timeout comparisons
- The deadline is calculated based on protocol-specific timeout values and the timestamp provided to SetSentTime()

**GetReadyTime(index int) time.Time**:
- Precondition: `index < GetRequestCount()`
- Precondition: request state is ConfigRequestPausing
- Returns the absolute time when the GPS receiver will be ready for the next configuration command
- The returned time is non-zero and includes monotonic time for accurate comparisons
- The time is calculated as ACK reception time + pause duration when the request transitions to ConfigRequestPausing state

**GetError(index int) error**:
- Precondition: `index < GetRequestCount()`
- Precondition: request state is ConfigRequestFailed
- Returns the error details for a failed request
- Error details may include protocol-specific failure reasons, NACK codes, or timeout information to aid in debugging configuration issues
- The caller should use this error for logging, user feedback, or determining recovery strategies when a request permanently fails

**SetSentTime(index int, tSent time.Time)**:
- Precondition: `index < GetRequestCount()`
- Records when the request packet was transmitted to the GPS receiver
- State effect: ConfigRequestReadyToSend → ConfigRequestAwaitingResponse (if acknowledgment or response expected) or ConfigRequestSucceeded (if no response expected)
- State effect: ConfigRequestMayResend → ConfigRequestAwaitingResponse (client chooses to retry)
- The timestamp is used for timeout calculations and protocol timing requirements

**SetTimedOut(index int)**:
- Precondition: `index < GetRequestCount()`
- Precondition: request state is ConfigRequestAwaitingResponse
- Marks a request as timed out
- State effect: ConfigRequestAwaitingResponse → ConfigRequestMayResend
- This should be called when no response is received by the deadline
- The client can then decide whether to retry or abandon the request

**SetWontResend(index int)**:
- Precondition: `index < GetRequestCount()`
- Precondition: request state is ConfigRequestMayResend
- Marks a request as permanently failed because the client decides not to retry
- State effect: ConfigRequestMayResend → ConfigRequestFailed
- This should be called when the client decides not to retry a timed-out request, typically after reaching a maximum retry count
- Failed requests trigger configuration abort and recovery procedures

### Key Design Principles

1. **Clean Request Interface**: Each `ConfigRequest` provides a clean, stateful handle to a configuration request:
   - Combines request data (packet, speed change) with execution state (sent time, current state)
   - No need to pass indices around - the request object encapsulates everything
   - Type-safe access to state-dependent information (deadlines, errors)
   - Clear method names that reflect the request lifecycle

2. **Automatic State Management**: The Configurator automatically handles state transitions based on received packets:
   - When an ACK is received, the request transitions to `ConfigRequestSucceeded`
   - When a NACK is received, the request transitions to `ConfigRequestFailed`
   - When a poll response is received, the request transitions to `ConfigRequestSucceeded`
   - For certain errors, the Configurator may transition from `ConfigRequestAwaitingResponse` directly to `ConfigRequestFailed` (instead of allowing retries)
   - The caller no longer needs to call `FindAck()` and `Done()` - this happens internally

### Benefits

1. **Better Encapsulation**: Request details and state are encapsulated in a clean object interface
2. **Simplified Caller Logic**: No need to call `FindAck()` and `Done()` - packet processing handles state transitions automatically
3. **Type Safety**: Methods like `GetResponseDeadline()` can only be called on requests in appropriate states
4. **Clear State Machine**: Explicit state transitions are easy to understand and test
5. **Sequential Processing**: `ConfigRequestNotReady` ensures requests are processed in order
6. **Natural Retry Handling**: `ConfigRequestMayResend` state gives client explicit control over retry decisions
7. **Protocol Independence**: The interface works for any protocol implementation
8. **Easier Testing**: Request objects can be easily mocked and tested independently

### State Transitions

- `ConfigRequestNotReady` → `ConfigRequestReadyToSend` (when previous request succeeds)
- `ConfigRequestReadyToSend` → `ConfigRequestAwaitingResponse` | `ConfigRequestSucceeded` (via SetSentTime)
- `ConfigRequestAwaitingResponse` → `ConfigRequestPausing` | `ConfigRequestSucceeded` | `ConfigRequestFailed` (via packet processing)
- `ConfigRequestAwaitingResponse` → `ConfigRequestMayResend` (via SetTimedOut)
- `ConfigRequestAwaitingResponse` → `ConfigRequestFailed` (via Configurator decision - no retry allowed)
- `ConfigRequestPausing` → `ConfigRequestSucceeded` (when pause duration elapses)
- `ConfigRequestMayResend` → `ConfigRequestAwaitingResponse` (via SetSentTime - client chooses retry)
- `ConfigRequestMayResend` → `ConfigRequestFailed` (via SetWontResend)
- `ConfigRequestNotReady` → `ConfigRequestSkipped` (when dependency fails)

### Implementation Notes

Implementations can use various internal structures to track requests. The UBX implementation uses a single slice of structs containing the request and its state, but this is an implementation detail. The key requirement is that the gpsprot interface only exposes indices, never ConfigRequest objects.

### ConfigDirector

The `ConfigDirector` serves as a high-level coordinator that wraps the low-level `Configurator` interface to provide significant benefits:

**Key Benefits:**
1. **Enhanced Testability** - Both production code (`gpscfg`) and test code (`replayer`) can use the same `ConfigDirector`, eliminating duplicate logic and ensuring tests exercise the same paths as production
2. **Simplified Client Code** - Clients get simple `ConfigAction` instructions instead of managing complex state transitions, retry logic, and timing coordination
3. **Protocol Independence** - The same `ConfigDirector` works with any protocol implementation (UBX, NMEA, etc.) since it operates on the abstract `Configurator` interface
4. **Centralized Complexity** - All the complex coordination logic (retry handling, timeout management, request windowing) is centralized in one place rather than duplicated across clients

**How It Works:**
The `ConfigDirector` uses an "active window" of requests (defined by start and end indices) to efficiently manage batching and retries. It examines current request states and directs the next action the client should take, without executing the actions itself.

```go
type ConfigActionType int
const (
    ConfigActionSendRequest ConfigActionType = iota
    ConfigActionCheckTimeout  // Client should check for timeouts
    ConfigActionWaitUntil
    ConfigActionError        // Configuration error occurred
)

type ConfigAction struct {
    Type     ConfigActionType
    Index    int           // For ConfigActionSend/CheckTimeout
    Packet   []byte       // For ConfigActionSendRequest
    Speed    int          // For ConfigActionSendRequest (0 if no change)
    Deadline time.Time    // For ConfigActionWaitUntil, ConfigActionCheckTimeout
    Error    error        // For ConfigActionError
}

type ConfigDirector struct {
    cfgtor      Configurator
    startIndex  int          // First request not in a final state
    endIndex    int          // First request not yet discovered and ready
    retries     []int        // Track retries per request index (grown as needed)
    maxRetries  int
    ErrorCount  int          // Number of ConfigActionError actions yielded
}

func NewConfigDirector(cfgtor Configurator, maxRetries int) *ConfigDirector {
    return &ConfigDirector{
        cfgtor:      cfgtor,
        startIndex:  0,
        endIndex:    0,
        retries:     nil, // Grown as needed
        maxRetries:  maxRetries,
    }
}

// Actions returns an iterator over configuration actions
func (cd *ConfigDirector) Actions() iter.Seq[ConfigAction] {
    return func(yield func(ConfigAction) bool) {
        for {
            // Try to generate more requests
            if err := cd.cfgtor.GenerateRequests(); err != nil {
                cd.ErrorCount++
                if !yield(ConfigAction{Type: ConfigActionError, Error: err}) { return }
                return
            }
            
            // Get current request count and completion status
            count, complete := cd.cfgtor.GetRequestCount()
            
            // Check if we're done early
            if complete && cd.startIndex >= count {
                return // Configuration complete - no more actions
            }
            
            // Update window bounds based on actual request states, yielding errors for failed requests
            if !cd.updateWindow(count, yield) {
                return
            }
            
            // Process requests in the active window
            var earliestDeadline time.Time
            for i := cd.startIndex; i < cd.endIndex && i < count; i++ {
                req := cd.cfgtor.Request(i)
                state := req.GetState()
                switch state {
                case ConfigRequestAwaitingResponse:
                    deadline := req.GetResponseDeadline()
                    if !yield(ConfigAction{
                        Type:     ConfigActionCheckTimeout,
                        Index:    i,
                        Deadline: deadline,
                    }) { return }
                    
                    // Track earliest deadline for WaitUntil action
                    if earliestDeadline.IsZero() || deadline.Before(earliestDeadline) {
                        earliestDeadline = deadline
                    }
                    
                case ConfigRequestPausing:
                    readyTime := req.GetReadyTime()
                    
                    // Track earliest ready time for WaitUntil action
                    if earliestDeadline.IsZero() || readyTime.Before(earliestDeadline) {
                        earliestDeadline = readyTime
                    }
                    
                case ConfigRequestMayResend:
                    // Handle retry logic
                    cd.ensureRetriesSize(i + 1)
                    cd.retries[i]++
                    if cd.retries[i] >= cd.maxRetries {
                        req.SetWontResend()
                        break // Move to next request in loop
                    }
                    fallthrough
                    
                case ConfigRequestReadyToSend:
                    if !yield(ConfigAction{
                        Type:   ConfigActionSendRequest,
                        Index:  i,
                        Packet: req.GetPacket(),
                        Speed:  req.GetSpeedChangeAfter(),
                    }) { return }
                }
            }
            
            
            // If we have any awaiting requests, yield wait action
            if !earliestDeadline.IsZero() {
                if !yield(ConfigAction{Type: ConfigActionWaitUntil, Deadline: earliestDeadline}) { return }
            }
            
        }
    }
}

// updateWindow updates the window bounds based on actual request states
func (cd *ConfigDirector) updateWindow(count int, yield func(ConfigAction) bool) bool {
    // Advance startIndex past completed requests, yielding errors for failed ones
    for cd.startIndex < count {
        req := cd.cfgtor.Request(cd.startIndex)
        state := req.GetState()
        if state == ConfigRequestSucceeded || state == ConfigRequestSkipped {
            cd.startIndex++
        } else if state == ConfigRequestFailed {
            // Yield error for this failed request exactly once as we advance past it
            cd.ErrorCount++
            err := req.GetError()
            if !yield(ConfigAction{Type: ConfigActionError, Error: err}) { 
                return false 
            }
            cd.startIndex++
        } else {
            break
        }
    }
    
    // Expand endIndex to include actionable requests
    for cd.endIndex < count {
        req := cd.cfgtor.Request(cd.endIndex)
        state := req.GetState()
        if state == ConfigRequestReadyToSend || state == ConfigRequestAwaitingResponse || 
           state == ConfigRequestPausing || state == ConfigRequestMayResend {
            cd.endIndex++
        } else {
            break
        }
    }
    
    return true
}

// ensureRetriesSize ensures the retries slice is large enough for the given size
func (cd *ConfigDirector) ensureRetriesSize(size int) {
    for len(cd.retries) < size {
        cd.retries = append(cd.retries, 0)
    }
}
```

### Usage in internal/gpscfg/gpscfg.go

This would replace the existing `msgHandler.configure()` method:

```go
func (mh *msgHandler) configure(ctx context.Context, prot gpsprot.PacketExchanger, target *gpsprot.ConfigTarget, port gpsio.OutPort) (*gpsprot.ConfigProps, *gpsprot.ReceiverInfo, error) {
    cfgtor, err := prot.Configure(target)
    if err != nil {
        return nil, nil, err
    }
    
    director := gpsprot.NewConfigDirector(cfgtor, maxTries)
    var knownErr error // error that we know how to handle

    for action := range director.Actions() {
        switch action.Type {
        case gpsprot.ConfigActionSendRequest:
            // Capture send time before write - ACK matching requires sentTime <= ackTime
            tSend := time.Now()
            var err error
            if action.Speed != 0 {
                // Serial port specific - send then change speed
                if serPort, ok := port.(*gpsio.SerialConn); ok {
                    _, err = serPort.WriteThenChangeSpeed(action.Packet, action.Speed)
                } else {
                    err = fmt.Errorf("speed change requested but port is not serial")
                }
            } else {
                _, err = port.Write(action.Packet)
            }
            if err != nil {
                return nil, nil, fmt.Errorf("failed to send configuration packet: %w", err)
            }
            req := cfgtor.Request(action.Index)
            req.SetSentTime(tSend)
            
        case gpsprot.ConfigActionCheckTimeout:
            // Check if this specific request has timed out
            if time.Now().After(action.Deadline) {
                req := cfgtor.Request(action.Index)
                req.SetTimedOut()
            }
            
        case gpsprot.ConfigActionWaitUntil:
            select {
            case <-ctx.Done():
                return nil, nil, ctx.Err()
            case <-time.After(time.Until(action.Deadline)):
                // Continue to next iteration to check for state changes
            case packet, ok := <-mh.packetCh:
                if !ok {
                    return nil, nil, mh.packetChClosed(ctx)
                }
                mh.packet(packet)
            }
            
        case gpsprot.ConfigActionError:
            if knownErr == nil {
                mh.lg.Warn("GPS configuration failed", "err", action.Error)
                knownErr = action.Error
            }
            // Continue to allow recovery operations
            
        }
    }
    
    // Iterator ended - configuration complete
    return cfgtor.ConfigProps(), cfgtor.ReceiverInfo(), knownErr
}
```


### Replay Testing Compatibility

The index-based design enhances replay testing:

**Preserved Determinism**: The Configurator remains completely deterministic. All timing information comes from recorded timestamps, and packet processing follows the same logic as the current system.

**Enhanced Test Clarity**: Instead of tracking complex FindAck/Done sequences, replay tests verify straightforward state transitions. Test assertions become clearer: "after feeding packet X, request 5 should be ConfigRequestSucceeded."

**Better Error Reproduction**: Failed scenarios are easier to reproduce because request states are explicit. Tests can verify exactly which requests succeeded, failed, or were skipped.

**Simplified Test Logic**: The replay harness becomes simpler because it no longer needs to simulate the caller's FindAck/Done logic - it just feeds packets and checks resulting states against expected values.

This design addresses all the main weaknesses while preserving testability and making protocol implementations straightforward to write and maintain.



### UBX Implementation Outline

Based on the existing code in `internal/ubx/ubxcfg.go`, here's how the UBX Configurator would be updated to implement the new interface:

**Step 1: Rename existing interface**
First, the existing `ConfigRequest` interface would be renamed to `requestOps` to represent request-type-specific operations:

```go
// internal/ubx/ubxcfg.go

// requestOps defines the operations available for different request types (unexported)
type requestOps interface {
    Packet() []byte
    ChangeSpeed() int
    Pause() time.Duration
    Ackable() bool
    AwaitingResponse(tSent time.Time) bool
    Done()
    ID() string
}

// configRequest is the UBX implementation of gpsprot.ConfigRequest
type configRequest struct {
    ops            requestOps                 // The request-specific operations (msgRequest, pollRequest, etc.)
    sentTime       time.Time                  // When the request was sent
    pauseStartTime time.Time                  // When the pause period started (for ConfigRequestPausing)
    state          gpsprot.ConfigRequestState // Current state
    err            error                      // Error details for failed requests
    awaitingAck    bool                       // true if we still need an ACK for this request
}

type Configurator struct {
    // Request tracking
    requests  []*configRequest       // All requests and their state
    nextIndex int                    // Index of first request that hasn't succeeded
    complete  bool                   // True when all steps have been processed

    // Existing fields remain
    ver       *Version
    tRead     map[bin.MsgID]time.Time
    raw       RawConfig
    origPrt   *bin.CfgPrt
    monGNSS   *monGNSS
    steps     []func(*Configurator) error
    stepIndex int
    target    *gpsprot.ConfigTarget
    survey    bool
}

// configRequest implements gpsprot.ConfigRequest
func (cr *configRequest) GetPacket() []byte {
    return cr.ops.Packet()
}

func (cr *configRequest) GetSpeedChangeAfter() int {
    return cr.ops.ChangeSpeed()
}

func (cr *configRequest) GetState() gpsprot.ConfigRequestState {
    return cr.state
}

func (cr *configRequest) GetResponseDeadline() time.Time {
    if cr.state != gpsprot.ConfigRequestAwaitingResponse {
        panic("GetResponseDeadline called in wrong state")
    }
    // Response deadline is determined by the request type and protocol rules
    return cr.sentTime.Add(1500 * time.Millisecond)
}

func (cr *configRequest) GetReadyTime() time.Time {
    if cr.state != gpsprot.ConfigRequestPausing {
        panic("GetReadyTime called in wrong state")
    }
    // Return when the receiver will be ready (pause start time + pause duration)
    return cr.pauseStartTime.Add(cr.ops.Pause())
}

func (cr *configRequest) GetError() error {
    if cr.state != gpsprot.ConfigRequestFailed {
        panic("GetError called in wrong state")
    }
    return cr.err
}

func (cr *configRequest) SetSentTime(tSent time.Time) {
    cr.sentTime = tSent
    cr.awaitingAck = cr.ops.Ackable()
    
    // Check if already complete (no ACK needed and no response needed)
    if !cr.awaitingAck && !cr.ops.AwaitingResponse(tSent) {
        cr.state = gpsprot.ConfigRequestSucceeded
        cr.ops.Done()
    } else {
        cr.state = gpsprot.ConfigRequestAwaitingResponse
    }
}

func (cr *configRequest) SetTimedOut() {
    if cr.state != gpsprot.ConfigRequestAwaitingResponse {
        panic("SetTimedOut called in wrong state")
    }
    cr.state = gpsprot.ConfigRequestMayResend
}

func (cr *configRequest) SetWontResend() {
    if cr.state != gpsprot.ConfigRequestMayResend {
        panic("SetWontResend called in wrong state")
    }
    cr.state = gpsprot.ConfigRequestFailed
    cr.err = fmt.Errorf("no response to request")
}

// Implementation of new interface methods
func (c *Configurator) GenerateRequests() error {
    // First, handle state transitions and sequential progression
    c.updateRequestStates()
    c.advanceSequentialProgress()
    
    // Then generate new requests lazily using the existing step-based approach
    initialRequestCount := len(c.requests)
    
    for c.stepIndex < len(c.steps) {
        err := c.steps[c.stepIndex](c)
        c.stepIndex++
        if err != nil {
            return err
        }
        
        // If we generated new requests, return to allow processing them
        // before generating more (subsequent requests may depend on results)
        if len(c.requests) > initialRequestCount {
            return nil
        }
    }
    
    c.complete = true
    return nil
}

// updateRequestStates handles automatic state transitions (e.g., pause completion)
func (c *Configurator) updateRequestStates() {
    for i := range c.requests {
        rs := &c.requests[i]
        
        // Check if pausing request has completed its pause duration
        if rs.state == gpsprot.ConfigRequestPausing {
            pauseDuration := rs.request.Pause()
            if time.Since(rs.pauseStartTime) >= pauseDuration {
                rs.state = gpsprot.ConfigRequestSucceeded
                rs.request.Done()
            }
        }
    }
}

// advanceSequentialProgress advances nextIndex and makes next requests ready
func (c *Configurator) advanceSequentialProgress() {
    for c.nextIndex < len(c.requests) {
        rs := &c.requests[c.nextIndex]
        
        // If current request succeeded, advance to next
        if rs.state == gpsprot.ConfigRequestSucceeded {
            c.nextIndex++
            // Make next request ready if it exists
            if c.nextIndex < len(c.requests) {
                c.requests[c.nextIndex].state = gpsprot.ConfigRequestReadyToSend
            }
        } else if rs.state == gpsprot.ConfigRequestFailed {
            // Handle failed request - trigger recovery logic that was formerly in c.stop()
            c.handleFailedRequest(c.nextIndex)
            break // Stop sequential progress after failure
        } else {
            break // Current request not ready to advance
        }
    }
}

// handleFailedRequest handles recovery logic when a request fails
func (c *Configurator) handleFailedRequest(index int) {
    // Recovery logic formerly in c.stop() method
    // This could include marking dependent requests as skipped, 
    // initiating recovery procedures, etc.
    // Implementation depends on protocol-specific recovery strategy
}

func (c *Configurator) Request(index int) gpsprot.ConfigRequest {
    if index >= len(c.requests) {
        panic("index >= GetRequestCount()")
    }
    return c.requests[index]
}

// Modified addRequest to work with new structure
func (c *Configurator) addRequest(ops requestOps) error {
    state := gpsprot.ConfigRequestNotReady
    // First request starts as ready
    if len(c.requests) == 0 {
        state = gpsprot.ConfigRequestReadyToSend
    }
    
    c.requests = append(c.requests, &configRequest{
        ops:         ops,
        state:       state,
        awaitingAck: false,  // Will be set when request is sent
    })
    return nil
}

// Process incoming packets - update request status based on ACKs
func (c *Configurator) processMsg(msg bin.Msg, t time.Time) (bool, error) {
    switch mt := msg.(type) {
    case *bin.AckAck:
        c.processAckNak(mt.MsgID, true, t)
        return true, nil
        
    case *bin.AckNak:
        c.processAckNak(mt.MsgID, false, t)
        return true, nil
        
    case *bin.MonGnss:
        c.tRead[mt.ID()] = t
        c.monGNSS = c.newMonGNSS(mt)
        c.checkPollResponses(t)
        return true, nil
    }
    
    // Handle other message types as before
    mid := msg.ID()
    if mid.CfgClass() {
        c.tRead[mid] = t
        _, err := c.raw.AddMsg(msg)
        c.checkPollResponses(t)
        return err == nil, err
    }
    
    return false, nil
}

// processAckNak handles both positive and negative acknowledgments
func (c *Configurator) processAckNak(msgID bin.MsgID, ok bool, t time.Time) {
    // Find the request that matches this ACK/NACK
    for i := 0; i < len(c.requests); i++ {
        cr := c.requests[i]
        if cr.state == gpsprot.ConfigRequestAwaitingResponse && cr.awaitingAck {
            packet := cr.ops.Packet()
            if bin.PacketMsgId(packet) == msgID && !t.Before(cr.sentTime) {
                if ok {
                    cr.awaitingAck = false  // ACK received
                    cr.checkComplete(t)     // Pass ACK reception time
                } else {
                    cr.state = gpsprot.ConfigRequestFailed
                    cr.err = fmt.Errorf("GPS receiver sent NACK for request %s", cr.ops.ID())
                    // Note: Recovery logic now handled by GenerateRequests()
                }
                break
            }
        }
    }
}

// checkPollResponses checks if any awaiting poll requests are now satisfied
func (c *Configurator) checkPollResponses(t time.Time) {
    for i := 0; i < len(c.requests); i++ {
        c.requests[i].checkComplete(t)
    }
}

// checkComplete determines if a request is fully complete (both ACK and response received as needed)
// The ackTime parameter is the time when the ACK was actually received
func (cr *configRequest) checkComplete(ackTime time.Time) {
    if cr.state == gpsprot.ConfigRequestAwaitingResponse && 
       !cr.awaitingAck && !cr.ops.AwaitingResponse(cr.sentTime) {
        // Check if we need to pause before marking as succeeded
        if cr.ops.Pause() > 0 {
            cr.state = gpsprot.ConfigRequestPausing
            cr.pauseStartTime = ackTime  // Pause starts from ACK reception time
        } else {
            cr.state = gpsprot.ConfigRequestSucceeded
            cr.ops.Done()
        }
    }
}
```

The key changes from the existing implementation:

1. **New interface compliance**: Implements GenerateRequests(), GetRequestCount(), and updated method signatures
2. **Panic on violations**: All methods panic on invalid index or wrong state as specified
3. **Error tracking**: requestState includes error field for failed requests  
4. **State-based access**: GetPacket/GetSpeedChangeAfter work in Ready/MayResend/Failed states for error reporting
5. **Sequential processing**: Only the request at nextIndex can be Ready; others are NotReady
6. **Lazy generation**: GenerateRequests() replaces the lazy generation previously done in GetState()
7. **Updated state names**: Uses ConfigRequestFailed consistently throughout
8. **Method renaming**: SetFailed becomes SetWontResend with proper state preconditions
9. **Method renaming**: GetDeadline becomes GetResponseDeadline for clarity

### Replay Test Implementation

Here's how `replayer.run()` from `internal/gpscmd/replay_test.go` can be rewritten using ConfigDirector:

```go
func (r *replayer) run() {
    // Initialize packet processors like gpscfg does (unchanged)
    for _, pp := range r.packetProcs {
        pp.SetMsgHandler(r)
        if r.px == nil {
            r.px = pp.CreatePacketExchanger()
        }
    }

    // Probe phase (unchanged)
    r.feedUpTo(0)
    if len(r.test.outPackets) == 0 {
        return
    }
    probePacket := r.px.ProbePacket()
    expected := r.test.outPackets[0]
    if string(probePacket) != expected.Data() {
        r.t.Errorf("probe packet mismatch")
    }
    r.outIdx++
    
    r.feedUpTo(1)
    if !r.px.ProbeOK() {
        r.t.Error("probe did not succeed after feeding initial packets")
        return
    }

    // Create configurator and director
    var err error
    r.cfgtor, err = r.px.Configure(r.target)
    if err != nil {
        r.t.Fatal(err)
    }
    
    director := gpsprot.NewConfigDirector(r.cfgtor, maxTries)

    // Process configuration using director
    for action := range director.Actions() {
        switch action.Type {
        case gpsprot.ConfigActionSendRequest:
            // Verify output packet matches expected
            if r.outIdx >= len(r.test.outPackets) {
                r.t.Fatalf("unexpected request, no more output packets")
            }

            expected := r.test.outPackets[r.outIdx]
            if !r.packetsEqual("", action.Packet, expected) {
                r.t.Errorf("packet mismatch")
            }
            r.outIdx++
            
            // Mark as sent using recorded timestamp
            req := r.cfgtor.Request(action.Index)
            req.SetSentTime(time.Time(expected.T))
            
        case gpsprot.ConfigActionCheckTimeout:
            // Use recorded packet timestamps for deterministic timeout checking
            // Find the last input packet timestamp we've processed
            var lastProcessedTime time.Time
            if r.inIdx > 0 {
                lastProcessedTime = time.Time(r.test.inPackets[r.inIdx-1].T)
            }
            
            // Check timeout against recorded timeline
            if lastProcessedTime.After(action.Deadline) {
                req := r.cfgtor.Request(action.Index)
                req.SetTimedOut()
            }
            
        case gpsprot.ConfigActionWaitUntil:
            // Feed packets until we reach the deadline or get the response we need
            r.feedUpToTime(action.Deadline)
            
        case gpsprot.ConfigActionError:
            if r.configErr == nil {
                r.configErr = action.Error
            }
            // Continue to allow recovery operations
            
        }
    }

    if r.outIdx < len(r.test.outPackets) {
        r.t.Fatalf("failed to generate all output packets, got %d, want %d",
            r.outIdx, len(r.test.outPackets))
    }
}

// Helper method to feed packets up to a specific time
func (r *replayer) feedUpToTime(deadline time.Time) {
    for r.inIdx < len(r.test.inPackets) {
        p := r.test.inPackets[r.inIdx]
        if time.Time(p.T).After(deadline) {
            break // Don't feed packets beyond deadline
        }
        
        r.inIdx++
        if p.Tag == "" {
            continue // Skip invalid packets
        }
        
        pp, ok := r.packetProcs[p.Tag]
        if !ok {
            r.t.Errorf("no processor for tag %s", p.Tag)
            continue
        }
        
        _, err := pp.ProcessPacket(p.Data(), time.Time(p.T))
        if err != nil {
            r.t.Errorf("error processing packet: %v", err)
        }
    }
}
```
