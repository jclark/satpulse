package gpsprot

import (
	"iter"
	"time"
)

// ConfigProtocol2 manages the processing and generation of packets for a GPS receiver,
// replacing the old PacketExchanger interface with improved multi-format support.
//
// Methods do not send or receive packets themselves.
// ConfigProtocol2 embeds NativeMsgHandler to process protocol-specific messages directly.
type ConfigProtocol2 interface {
	NativeMsgHandler

	// ProbePacket returns a packet to be sent to the GPS receiver for probing.
	ProbePacket() []byte

	// ProbeOK returns true when a message has been received indicating the GPS receiver is responding.
	ProbeOK() bool

	// Configure2 creates a Configurator2 for the given configuration target.
	// The returned Configurator2 will receive packets via NativeMsg calls on this ConfigProtocol2.
	Configure2(target *ConfigTarget) (Configurator2, error)
}

// Configurator2 manages the generation and interpretation of configuration-related packets
// with improved state management and multi-response query support.
//
// The Configurator maintains a slice of requests and can generate additional requests lazily.
// It processes incoming packets to automatically update request states.
// No I/O, no timing - purely deterministic state management for testability.
type Configurator2 interface {
	// ConfigProps returns the current configuration of the GPS receiver.
	// Should be called after configuration completes to see what was achieved.
	ConfigProps() *ConfigProps

	// ReceiverInfo returns static information about the GPS receiver.
	ReceiverInfo() *ReceiverInfo

	// GenerateRequests attempts to generate more requests, potentially increasing the slice size.
	// This is the only method that can increase the slice size.
	// Generation is lazy - it may generate only some requests if later requests depend on earlier results.
	// Should be called when the client needs more requests than are currently available.
	// Returns an error if request generation fails.
	GenerateRequests() error

	// GetRequestCount returns the current number of requests and whether the slice is complete.
	// When bool is true, the slice count will never increase.
	// When bool is false, the slice count may increase after calling GenerateRequests().
	GetRequestCount() (count int, complete bool)

	// Request returns the ConfigRequest2 at the given index.
	// Precondition: index < count from GetRequestCount()
	// Panics if index is out of bounds.
	Request(index int) ConfigRequest2
}

// ConfigRequest2 represents a configuration request with state-based lifecycle management.
// Each request encapsulates both the request data and its execution state.
//
// State Transitions:
// - Client-initiated transitions occur via Set* method calls
// - Automatic transitions occur from packet processing within the Configurator
// - See ConfigRequestState documentation for detailed state semantics
//
// All precondition failures result in panics. All Get* methods are side-effect free.
type ConfigRequest2 interface {
	// GetPacket returns the packet bytes for this request.
	// Precondition: request state is ConfigRequestReadyToSend, ConfigRequestMayResend, or ConfigRequestFailed
	// The returned packet is ready to transmit to the GPS receiver without modification.
	// For ConfigRequestMayResend and ConfigRequestFailed states, this is useful for error reporting.
	GetPacket() []byte

	// GetSpeedChangeAfter returns the new baud rate to configure after sending this request.
	// Precondition: same states as GetPacket
	// Returns 0 if no speed change is required.
	// Speed changes are protocol-specific operations that must be handled by the client
	// after the packet is sent but before waiting for acknowledgment.
	GetSpeedChangeAfter() int

	// GetState returns the current state of this request.
	// This is the primary method for tracking configuration progress.
	GetState() ConfigRequestState

	// GetResponseDeadline returns the absolute time by which response packets are expected.
	// Precondition: request state is ConfigRequestAwaitingResponse or ConfigRequestMaybeComplete
	// The returned time is non-zero and includes monotonic time for accurate timeout comparisons.
	// For ConfigRequestAwaitingResponse: deadline for receiving first/only response (based on SetSentTime timestamp)
	// For ConfigRequestMaybeComplete: deadline for receiving next response in burst (based on last response time + idle period)
	GetResponseDeadline() time.Time

	// GetReadyTime returns the absolute time when the GPS receiver will be ready for the next command.
	// Precondition: request state is ConfigRequestPausing
	// The returned time is non-zero and includes monotonic time for accurate comparisons.
	// The time is calculated as ACK reception time + pause duration when transitioning to ConfigRequestPausing.
	GetReadyTime() time.Time

	// GetError returns the error details for a failed request.
	// Precondition: request state is ConfigRequestFailed
	// Error details may include protocol-specific failure reasons, NACK codes, or timeout information.
	// The caller should use this error for logging, user feedback, or determining recovery strategies.
	GetError() error

	// SetSentTime records when the request packet was transmitted to the GPS receiver.
	// Precondition: request state is ConfigRequestReadyToSend or ConfigRequestMayResend
	// State effect: ConfigRequestReadyToSend → ConfigRequestAwaitingResponse (if response expected)
	//               or ConfigRequestSucceeded (if no response expected)
	// State effect: ConfigRequestMayResend → ConfigRequestAwaitingResponse (client chooses to retry)
	// The timestamp is used for timeout calculations and protocol timing requirements.
	SetSentTime(tSent time.Time)

	// SetResponseDeadlinePassed notifies that the response deadline has passed.
	// Precondition: request state is ConfigRequestAwaitingResponse or ConfigRequestMaybeComplete
	// State effect: ConfigRequestAwaitingResponse → ConfigRequestMayResend (timeout, can retry)
	// State effect: ConfigRequestMaybeComplete → ConfigRequestSucceeded (idle period over, no more responses expected)
	// The client should call this when GetResponseDeadline() time has passed.
	SetResponseDeadlinePassed()

	// SetWontResend marks a request as permanently failed because the client decides not to retry.
	// Precondition: request state is ConfigRequestMayResend
	// State effect: ConfigRequestMayResend → ConfigRequestFailed
	// This should be called when the client decides not to retry a timed-out request,
	// typically after reaching a maximum retry count.
	SetWontResend()
}

// ConfigRequestState represents the current state of a configuration request.
type ConfigRequestState int

const (
	// ConfigRequestNotReady means this request cannot be sent yet because
	// some earlier requests are not yet in the Succeeded state.
	ConfigRequestNotReady ConfigRequestState = iota

	// ConfigRequestReadyToSend means this request is ready to be sent to the GPS receiver.
	ConfigRequestReadyToSend

	// ConfigRequestAwaitingResponse means the request has been sent and the Configurator
	// is expecting to receive one or more packets in response from the receiver.
	ConfigRequestAwaitingResponse

	// ConfigRequestMaybeComplete means the request has received at least one response packet
	// and may be waiting for more. This state is used for queries that can produce multiple
	// response packets where the total number is not known in advance. The request will
	// transition to Succeeded when the response deadline passes without new responses.
	ConfigRequestMaybeComplete

	// ConfigRequestPausing means the request received its acknowledgment/response and is
	// waiting for a protocol-specified pause duration before the next request can be sent.
	ConfigRequestPausing

	// ConfigRequestSucceeded means the request completed successfully.
	ConfigRequestSucceeded

	// ConfigRequestMayResend means the request timed out waiting for a response and is
	// eligible for retry. The client must decide whether to retry (via SetSentTime)
	// or abandon (via SetWontResend).
	ConfigRequestMayResend

	// ConfigRequestFailed means the request was sent but did not succeed and cannot be retried.
	ConfigRequestFailed

	// ConfigRequestSkipped means this request was skipped because an earlier request did not succeed.
	ConfigRequestSkipped
)

// ConfigDirector coordinates configuration operations by providing high-level actions to clients.
// It wraps a Configurator2 to provide:
// - Automatic retry management
// - Request windowing for efficient batching
// - Simplified client interface via ConfigAction instructions
// - Protocol-independent orchestration logic
//
// The same ConfigDirector can be used by both production code (gpscfg) and test code (replayer),
// ensuring consistent behavior and eliminating duplicate logic.
type ConfigDirector struct {
	cfgtor     Configurator2
	startIndex int   // First request not in a final state
	endIndex   int   // First request not yet discovered and ready
	retries    []int // Track retries per request index
	maxRetries int
	ErrorCount int // Number of ConfigActionError actions yielded
}

// ConfigActionType specifies the type of action the client should take.
type ConfigActionType int

const (
	// ConfigActionSendRequest means the client should send a configuration packet.
	ConfigActionSendRequest ConfigActionType = iota

	// ConfigActionCheckTimeout means the client should check if a request has timed out.
	ConfigActionCheckTimeout

	// ConfigActionWaitUntil means the client should wait until the specified deadline.
	ConfigActionWaitUntil

	// ConfigActionError means a configuration error occurred.
	ConfigActionError
)

// ConfigAction represents an action the client should take during configuration.
type ConfigAction struct {
	Type     ConfigActionType
	Index    int           // Request index for Send/CheckTimeout actions
	Packet   []byte        // Packet to send for SendRequest action
	Speed    int           // Speed change after send (0 if none)
	Deadline time.Time     // Deadline for WaitUntil/CheckTimeout actions
	Error    error         // Error details for Error action
}

// NewConfigDirector creates a new ConfigDirector for the given Configurator2.
// maxRetries specifies the maximum number of retry attempts for timed-out requests.
func NewConfigDirector(cfgtor Configurator2, maxRetries int) *ConfigDirector {
	return &ConfigDirector{
		cfgtor:     cfgtor,
		maxRetries: maxRetries,
	}
}

// Actions returns an iterator over configuration actions.
// The iterator yields ConfigAction instructions that the client should execute.
// The iterator ends when configuration is complete or fatally fails.
//
// The ConfigDirector manages an "active window" of requests to enable efficient
// batching and retry handling. It examines request states and determines the
// next action without executing the actions itself.
func (cd *ConfigDirector) Actions() iter.Seq[ConfigAction] {
	return func(yield func(ConfigAction) bool) {
		for {
			// Try to generate more requests
			if err := cd.cfgtor.GenerateRequests(); err != nil {
				cd.ErrorCount++
				if !yield(ConfigAction{Type: ConfigActionError, Error: err}) {
					return
				}
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
				switch req.GetState() {
				case ConfigRequestAwaitingResponse, ConfigRequestMaybeComplete:
					deadline := req.GetResponseDeadline()
					if !yield(ConfigAction{
						Type:     ConfigActionCheckTimeout,
						Index:    i,
						Deadline: deadline,
					}) {
						return
					}

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
					}) {
						return
					}
				}
			}

			// If we have any awaiting requests, yield wait action
			if !earliestDeadline.IsZero() {
				if !yield(ConfigAction{Type: ConfigActionWaitUntil, Deadline: earliestDeadline}) {
					return
				}
			} else if cd.startIndex >= cd.endIndex {
				// No deadline to wait for AND no actionable requests in window
				if cd.startIndex < count {
					// There are still requests but none are actionable
					// This means they're stuck in NotReady state - Configurator bug
					panic("Configurator bug: requests stuck in NotReady state with no way to progress")
				}
				// If startIndex >= count, all requests are done, we'll exit on next iteration
			}
		}
	}
}

// updateWindow updates the window bounds based on actual request states
func (cd *ConfigDirector) updateWindow(count int, yield func(ConfigAction) bool) bool {
	// Advance startIndex past completed requests, yielding errors for failed ones
advanceLoop:
	for cd.startIndex < count {
		req := cd.cfgtor.Request(cd.startIndex)
		switch req.GetState() {
		case ConfigRequestSucceeded, ConfigRequestSkipped:
			cd.startIndex++
		case ConfigRequestFailed:
			// Yield error for this failed request exactly once as we advance past it
			cd.ErrorCount++
			err := req.GetError()
			if !yield(ConfigAction{Type: ConfigActionError, Error: err}) {
				return false
			}
			cd.startIndex++
		default:
			// Stop at first non-final request
			break advanceLoop
		}
	}

	// Ensure endIndex is at least startIndex
	if cd.endIndex < cd.startIndex {
		cd.endIndex = cd.startIndex
	}

	// Expand endIndex to include actionable requests
expandLoop:
	for cd.endIndex < count {
		req := cd.cfgtor.Request(cd.endIndex)
		switch req.GetState() {
		case ConfigRequestReadyToSend, ConfigRequestAwaitingResponse,
			ConfigRequestMaybeComplete, ConfigRequestPausing, ConfigRequestMayResend:
			cd.endIndex++
		default:
			// Stop at first non-actionable request
			break expandLoop
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