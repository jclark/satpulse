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
    ConfigRequestMaybeComplete
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
    GetDeadline() time.Time  // valid when state is ConfigRequestAwaitingResponse or ConfigRequestMaybeComplete
    GetReadyTime() time.Time         // valid when state is ConfigRequestPausing
    GetError() error                 // valid when state is ConfigRequestFailed

    SetSentTime(tSent time.Time)
    SetDeadlinePassed()
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

**ConfigRequestMaybeComplete**: The request has received at least one response packet and may be waiting for more. This state is used for queries that can produce multiple response packets where the total number is not known in advance. The request will transition to Succeeded when the response deadline passes without new responses.

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
- `SetDeadlinePassed()`: ConfigRequestAwaitingResponse → ConfigRequestMayResend
- `SetDeadlinePassed()`: ConfigRequestMaybeComplete → ConfigRequestSucceeded
- `SetWontResend()`: ConfigRequestMayResend → ConfigRequestFailed

**Automatic transitions from packet processing:**
- Positive ACK received + pause needed: ConfigRequestAwaitingResponse → ConfigRequestPausing
- Positive ACK received + no pause: ConfigRequestAwaitingResponse → ConfigRequestSucceeded
- NACK received: ConfigRequestAwaitingResponse → ConfigRequestFailed  
- Expected poll response received + pause needed: ConfigRequestAwaitingResponse → ConfigRequestPausing
- Expected poll response received + no pause: ConfigRequestAwaitingResponse → ConfigRequestSucceeded
- First response received for multi-response query: ConfigRequestAwaitingResponse → ConfigRequestMaybeComplete
- Additional responses received: ConfigRequestMaybeComplete → ConfigRequestMaybeComplete (updates internal timing)
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

**GetDeadline(index int) time.Time**:
- Precondition: `index < GetRequestCount()`
- Precondition: request state is ConfigRequestAwaitingResponse or ConfigRequestMaybeComplete
- Returns the absolute time by which response packets are expected
- The returned time is non-zero and includes monotonic time for accurate timeout comparisons
- For ConfigRequestAwaitingResponse: deadline for receiving first/only response (based on SetSentTime timestamp)
- For ConfigRequestMaybeComplete: deadline for receiving next response in burst (based on last response time + idle period)

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

**SetDeadlinePassed(index int)**:
- Precondition: `index < GetRequestCount()`
- Precondition: request state is ConfigRequestAwaitingResponse or ConfigRequestMaybeComplete
- Notifies that the response deadline has passed
- State effect: ConfigRequestAwaitingResponse → ConfigRequestMayResend (timeout, can retry)
- State effect: ConfigRequestMaybeComplete → ConfigRequestSucceeded (idle period over, no more responses expected)
- The client should call this when GetDeadline() time has passed
- For AwaitingResponse, the client can then decide whether to retry or abandon
- For MaybeComplete, this indicates the response burst is considered complete

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
3. **Type Safety**: Methods like `GetDeadline()` can only be called on requests in appropriate states
4. **Clear State Machine**: Explicit state transitions are easy to understand and test
5. **Sequential Processing**: `ConfigRequestNotReady` ensures requests are processed in order
6. **Natural Retry Handling**: `ConfigRequestMayResend` state gives client explicit control over retry decisions
7. **Protocol Independence**: The interface works for any protocol implementation
8. **Easier Testing**: Request objects can be easily mocked and tested independently
9. **Multi-Response Query Support**: `ConfigRequestMaybeComplete` state properly handles queries that produce variable numbers of response packets

### State Transitions

- `ConfigRequestNotReady` → `ConfigRequestReadyToSend` (when previous request succeeds)
- `ConfigRequestReadyToSend` → `ConfigRequestAwaitingResponse` | `ConfigRequestSucceeded` (via SetSentTime)
- `ConfigRequestAwaitingResponse` → `ConfigRequestPausing` | `ConfigRequestSucceeded` | `ConfigRequestFailed` (via packet processing)
- `ConfigRequestAwaitingResponse` → `ConfigRequestMaybeComplete` (via packet processing - first response for multi-response query)
- `ConfigRequestAwaitingResponse` → `ConfigRequestMayResend` (via SetDeadlinePassed)
- `ConfigRequestAwaitingResponse` → `ConfigRequestFailed` (via Configurator decision - no retry allowed)
- `ConfigRequestMaybeComplete` → `ConfigRequestMaybeComplete` (via packet processing - additional responses)
- `ConfigRequestMaybeComplete` → `ConfigRequestSucceeded` (via SetDeadlinePassed - idle period over)
- `ConfigRequestPausing` → `ConfigRequestSucceeded` (when pause duration elapses)
- `ConfigRequestMayResend` → `ConfigRequestAwaitingResponse` (via SetSentTime - client chooses retry)
- `ConfigRequestMayResend` → `ConfigRequestFailed` (via SetWontResend)
- `ConfigRequestNotReady` → `ConfigRequestSkipped` (when dependency fails)

### Implementation Notes

Implementations can use various internal structures to track requests. The UBX implementation uses a single slice of structs containing the request and its state, but this is an implementation detail. 

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
  // ...
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

    for action := range director.Actions() {
        switch action.Type {
        case gpsprot.ConfigActionSendRequest:
           // send the packet and call req.SetSentTime
           // ...
        case gpsprot.ConfigActionCheckTimeout:
            // Check if this specific request's deadline has passed
            if time.Now().After(action.Deadline) {
                req := cfgtor.Request(action.Index)
                req.SetDeadlinePassed()
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



```go
// internal/ubx/ubxcfg.go

// Internal state enum to track detailed ACK/response lifecycle
type configRequestState int

const (
    stateNotReady              configRequestState = iota // Maps to ConfigRequestNotReady
    stateReadyToSend                                    // Maps to ConfigRequestReadyToSend
    stateAwaitingAck                                   // Maps to ConfigRequestAwaitingResponse
    stateAwaitingResponse                              // Maps to ConfigRequestAwaitingResponse
    stateAwaitingAckAndResponse                        // Maps to ConfigRequestAwaitingResponse
    statePausing                                       // Maps to ConfigRequestPausing
    stateMayResend                                     // Maps to ConfigRequestMayResend
    stateFailed                                        // Maps to ConfigRequestFailed
    stateSucceeded                                     // Maps to ConfigRequestSucceeded
    stateSkipped                                       // Maps to ConfigRequestSkipped
)

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
    ops            requestOps            // The request-specific operations (msgRequest, pollRequest, etc.)
    state          configRequestState    // Internal state for ACK/response tracking
    sentTime       time.Time             // When the request was sent
    pauseStartTime time.Time             // When the pause period started
    err            error                 // Error details for failed requests
}

type Configurator struct {
    // Request tracking
    requests     []*configRequest       // All requests and their state
    nextReadyIdx int                    // Index of first request that can be ready
    complete     bool                   // True when all steps have been processed

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


