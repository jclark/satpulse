package gpsprot

import (
	"encoding/json"
	"iter"
	"strings"
	"time"
)

// ConfigProtocol manages the processing and generation of packets for a GPS receiver,
// replacing the old PacketExchanger interface with improved multi-format support.
//
// Methods do not send or receive packets themselves.
// ConfigProtocol embeds NativeMsgHandler to process protocol-specific messages directly.
type ConfigProtocol interface {
	NativeMsgHandler

	// ProbePacket returns a packet to be sent to the GPS receiver for probing.
	ProbePacket() []byte

	// ProbeOK returns true when a message has been received indicating the GPS receiver is responding.
	ProbeOK() bool

	// Configure creates a Configurator for the given configuration target.
	// The returned Configurator will receive packets via NativeMsg calls on this ConfigProtocol.
	Configure(target *ConfigTarget) (Configurator, error)
}

// ConfigSupportFlags describes supported configuration options.
type ConfigSupportFlags uint32

const (
	ConfigSupportBand ConfigSupportFlags = 1 << iota
	ConfigSupportSpeed
	ConfigSupportSurvey
	ConfigSupportSurveyAcc
	ConfigSupportSurveyMsg
	ConfigSupportFixedPos
	ConfigSupportFixedPosAcc
	ConfigSupportRaw
	ConfigSupportRTCMMSM4
	ConfigSupportRTCMMSM7
	ConfigSupportRTCMBaseID
	ConfigSupportRTCMQZSS
	ConfigSupportPort
	ConfigSupportLast = ConfigSupportPort
	// ConfigSupportFull is every configuration flag set - the support of a
	// maximally capable receiver. A new flag joins it automatically, so a
	// backend may declare its support as ConfigSupportFull with the flags
	// it lacks cleared, and then gains future capabilities without being
	// edited.
	ConfigSupportFull = (ConfigSupportLast << 1) - 1
)

const ConfigSupportRTCMMSM = ConfigSupportRTCMMSM4 | ConfigSupportRTCMMSM7

var configSupportFlagNames = [...]struct {
	flag ConfigSupportFlags
	name string
}{
	{ConfigSupportBand, "band"},
	{ConfigSupportSpeed, "speed"},
	{ConfigSupportSurvey, "survey"},
	{ConfigSupportSurveyAcc, "surveyAcc"},
	{ConfigSupportSurveyMsg, "surveyMsg"},
	{ConfigSupportFixedPos, "fixedPos"},
	{ConfigSupportFixedPosAcc, "fixedPosAcc"},
	{ConfigSupportRaw, "raw"},
	{ConfigSupportRTCMMSM4, "rtcmMSM4"},
	{ConfigSupportRTCMMSM7, "rtcmMSM7"},
	{ConfigSupportRTCMBaseID, "rtcmBaseID"},
	{ConfigSupportRTCMQZSS, "rtcmQZSS"},
	{ConfigSupportPort, "port"},
}

// Items returns the supported configuration item names in stable order.
func (f ConfigSupportFlags) Items() []string {
	items := make([]string, 0, len(configSupportFlagNames))
	for _, entry := range configSupportFlagNames {
		if f&entry.flag != 0 {
			items = append(items, entry.name)
		}
	}
	return items
}

// String returns the supported configuration item names as a comma-separated list.
func (f ConfigSupportFlags) String() string {
	return strings.Join(f.Items(), ", ")
}

// MarshalJSON marshals the supported configuration items as a JSON array.
func (f ConfigSupportFlags) MarshalJSON() ([]byte, error) {
	return json.Marshal(f.Items())
}

// Configurator manages the generation and interpretation of configuration-related packets.
//
// The Configurator maintains a slice of ConfigRequest instances, each with its own state.
// It can generate additional requests lazily and modify existing request states.
// It processes incoming packets to automatically update request states.
// Configurators are time-agnostic: they never call time.Now() or track wall clock time.
// All time progression happens through client method calls, ensuring deterministic testability.
type Configurator interface {
	// ConfigProps returns the current configuration of the GPS receiver.
	// Should be called after configuration completes to see what was achieved.
	ConfigProps() *ConfigProps

	// ReceiverInfo returns static information about the GPS receiver.
	ReceiverInfo() *ReceiverInfo

	// ConfigSupport returns the configuration options this implementation supports.
	ConfigSupport() ConfigSupportFlags

	// GenerateRequests attempts to generate more requests, potentially increasing the slice size.
	// This is the only method that can increase the slice size.
	// May also change states of existing requests (see ConfigRequestState documentation).
	// Generation is lazy - it may generate only some requests if later requests depend on earlier results.
	// After successful return, either some request will be actionable or GetRequestCount will return complete=true.
	// Should be called when the client needs more requests than are currently available.
	// Returns an error if request generation fails.
	GenerateRequests() error

	// GetRequestCount returns the current number of requests and whether the slice is complete.
	// When bool is true, the slice count will never increase.
	// When bool is false, the slice count may increase after calling GenerateRequests().
	GetRequestCount() (count int, complete bool)

	// Request returns the ConfigRequest at the given index.
	// Precondition: index < count from GetRequestCount()
	// Panics if index is out of bounds.
	Request(index int) ConfigRequest
}

// ReceiverInfo provides static information about the GPS receiver.
type ReceiverInfo struct {
	Vendor         string      `json:"vendor"`        // receiver vendor (e.g., "u-blox")
	Firmware       string      `json:"firmware"`      // information about firmware; for u-blox, format would be e.g. "TIM 2.20 PROTVER 18.00"
	Hardware       string      `json:"hardware"`      // information about hardware; for u-blox, this is the model (e.g., "ZED-F9T")
	SupportedGNSS  GNSSSet     `json:"supportedGNSS"` // supported GNSS constellations
	VendorSpecific interface{} `json:"-"`             // vendor-specific information, excluded from JSON
}

// ConfigRequestState represents the current state of a configuration request.
// States are either actionable (client can/should take action) or not:
// - Actionable: ReadyToSend, AwaitingResponse, MaybeComplete, Pausing, MayResend
// - Not actionable: NotReady, Succeeded, Failed, Skipped
type ConfigRequestState int

const (
	// ConfigRequestNotReady means this request cannot be sent yet because
	// some earlier requests are not yet in the Succeeded state.
	// GenerateRequests may transition requests from this state to ReadyToSend.
	ConfigRequestNotReady ConfigRequestState = iota

	// ConfigRequestReadyToSend means this request is ready to be sent to the GPS receiver.
	ConfigRequestReadyToSend

	// ConfigRequestAwaitingResponse means the request has been sent and the Configurator
	// is expecting to receive one or more packets in response from the receiver.
	ConfigRequestAwaitingResponse

	// ConfigRequestMaybeComplete means the request may be complete but can still accept
	// further response packets. This state is used for queries that can produce multiple
	// response packets where the total number is not known in advance (entered when the
	// first response is received), and for requests that expect no response of their own
	// but may still absorb one (entered when the request is sent, see SetSentTime). The
	// request will transition to Succeeded when the response deadline passes without new
	// responses.
	ConfigRequestMaybeComplete

	// ConfigRequestPausing means the request received its acknowledgment/response and is
	// waiting for a protocol-specified pause duration before the next request can be sent.
	ConfigRequestPausing

	// ConfigRequestSucceeded means the request completed successfully.
	// This is a terminal state - the request will never transition to another state.
	ConfigRequestSucceeded

	// ConfigRequestMayResend means the request timed out waiting for a response and is
	// eligible for retry. The client must decide whether to retry (via SetSentTime)
	// or abandon (via SetWontResend).
	ConfigRequestMayResend

	// ConfigRequestFailed means the request was sent but did not succeed and cannot be retried.
	// This is a terminal state - the request will never transition to another state.
	ConfigRequestFailed

	// ConfigRequestSkipped means this request was skipped because an earlier request did not succeed.
	// GenerateRequests may transition requests from this state to ReadyToSend.
	// This is a terminal state - the request will never transition to another state.
	ConfigRequestSkipped
)

// ConfigRequest represents a configuration request with state-based lifecycle management.
// Each request encapsulates both the request data and its execution state.
//
// State Transitions:
// - Client-initiated transitions occur via Set* method calls
// - Automatic transitions occur from packet processing within the Configurator
// - See ConfigRequestState documentation for detailed state semantics
//
// Automatic state transitions from packet processing:
//   - ConfigRequestAwaitingResponse → ConfigRequestMaybeComplete (first of multiple responses received)
//   - ConfigRequestAwaitingResponse → ConfigRequestSucceeded (complete response received)
//   - ConfigRequestAwaitingResponse → ConfigRequestPausing (acknowledgment received, pause required)
//   - ConfigRequestAwaitingResponse → ConfigRequestFailed (negative acknowledgment received)
//   - ConfigRequestMaybeComplete → ConfigRequestSucceeded (response received that completes the request)
//   - ConfigRequestMaybeComplete → ConfigRequestFailed (negative acknowledgment received)
//
// All precondition failures result in panics. All Get* methods are side-effect free.
type ConfigRequest interface {
	// GetPacket returns the packet bytes for this request.
	// Precondition: request state is ConfigRequestReadyToSend, ConfigRequestMayResend, or ConfigRequestFailed
	// The returned packet is ready to transmit to the GPS receiver without modification.
	// For ConfigRequestMayResend and ConfigRequestFailed states, this is useful for error reporting.
	GetPacket() []byte

	// GetSpeedChangeAfter returns the new baud rate to configure after sending this request.
	// Precondition: request state is ConfigRequestReadyToSend, ConfigRequestMayResend, or ConfigRequestFailed
	// Returns 0 if no speed change is required.
	// Speed changes are protocol-specific operations that must be handled by the client
	// after the packet is sent but before waiting for acknowledgment.
	GetSpeedChangeAfter() int

	// GetState returns the current state of this request.
	// This is the primary method for tracking configuration progress.
	GetState() ConfigRequestState

	// GetDeadline returns the absolute time by which response packets are expected.
	// Precondition: request state is ConfigRequestAwaitingResponse, ConfigRequestMaybeComplete, or ConfigRequestPausing
	// The returned time is non-zero and includes monotonic time for accurate timeout comparisons.
	// For ConfigRequestAwaitingResponse: deadline for receiving first/only response (based on
	// SetSentTime timestamp); a protocol may stage deadlines (see SetDeadlinePassed)
	// For ConfigRequestMaybeComplete: deadline for receiving next response in burst (based on last response time + idle period)
	// For ConfigRequestPausing: deadline for when pause completes and GPS receiver is ready for next command
	GetDeadline() time.Time

	// GetError returns the error details for a failed request.
	// Precondition: request state is ConfigRequestFailed
	// Error details may include protocol-specific failure reasons, NACK codes, or timeout information.
	// The caller should use this error for logging, user feedback, or determining recovery strategies.
	GetError() error

	// SetSentTime records when the request packet was transmitted to the GPS receiver.
	// Precondition: request state is ConfigRequestReadyToSend or ConfigRequestMayResend
	// State transitions:
	//   - ConfigRequestReadyToSend → ConfigRequestAwaitingResponse (if response expected)
	//   - ConfigRequestReadyToSend → ConfigRequestSucceeded (if no response expected)
	//   - ConfigRequestReadyToSend → ConfigRequestMaybeComplete (if no response of its own
	//     is expected but the request may absorb one, e.g. the repeat of a speed change)
	//   - ConfigRequestMayResend → ConfigRequestAwaitingResponse (retry)
	// The timestamp is used for timeout calculations and protocol timing requirements.
	SetSentTime(tSent time.Time)

	// SetDeadlinePassed notifies that the response deadline has passed.
	// Precondition: request state is ConfigRequestAwaitingResponse, ConfigRequestMaybeComplete, or ConfigRequestPausing
	// State transitions:
	//   - ConfigRequestAwaitingResponse → ConfigRequestMayResend (timeout, can retry)
	//   - ConfigRequestAwaitingResponse → ConfigRequestAwaitingResponse (staged deadline:
	//     an intermediate deadline passed, triggering protocol-internal action such as
	//     generating a follow-up request; GetDeadline now returns a later deadline)
	//   - ConfigRequestMaybeComplete → ConfigRequestSucceeded (idle period over, no more responses expected)
	//   - ConfigRequestPausing → ConfigRequestSucceeded (pause duration elapsed, ready for next request)
	// The client should call this when GetDeadline() time has passed.
	SetDeadlinePassed()

	// SetWontResend marks a request as permanently failed because the client decides not to retry.
	// Precondition: request state is ConfigRequestMayResend
	// State transitions:
	//   - ConfigRequestMayResend → ConfigRequestFailed
	// This should be called when the client decides not to retry a timed-out request,
	// typically after reaching a maximum retry count.
	SetWontResend()

	// MaybeSpeedChangeSucceeded notifies the request that a valid packet was received at the given time.
	// Precondition: request state is ConfigRequestAwaitingResponse
	// State transitions:
	//   - ConfigRequestAwaitingResponse → ConfigRequestSucceeded (if conditions met)
	// The request will check if it's a speed change awaiting ACK and if sufficient time has passed
	// since sending to treat this as confirmation of success.
	MaybeSpeedChangeSucceeded(validPacketTime time.Time)
}

// ConfigDirector coordinates configuration operations by providing high-level actions to clients.
// It wraps a Configurator to provide:
// - Automatic retry management
// - Request windowing for efficient batching
// - Simplified client interface via ConfigAction instructions
// - Protocol-independent orchestration logic
//
// The same ConfigDirector can be used by both production code (gpscfg) and test code (replayer),
// ensuring consistent behavior and eliminating duplicate logic.
type ConfigDirector struct {
	cfgtor              Configurator
	startIndex          int              // First request not in a final state
	endIndex            int              // First request not yet discovered and ready
	retries             []int            // Track retries per request index
	speedChangeRequests map[int]struct{} // Set of request indices that are speed changes
	maxRetries          int
	ErrorCount          int       // Number of ConfigActionError actions yielded
	currentTime         time.Time // Current time as reported by client
	advanceCount        int       // Number of times AdvanceTimeTo has been called
}

// ConfigActionType specifies the type of action the client should take.
type ConfigActionType int

const (
	// ConfigActionSendRequest means the client should send a configuration packet.
	ConfigActionSendRequest ConfigActionType = iota

	// ConfigActionWaitUntil means the client should wait until the specified deadline.
	ConfigActionWaitUntil

	// ConfigActionError means a configuration error occurred.
	ConfigActionError
)

// ConfigAction represents an action the client should take during configuration.
type ConfigAction struct {
	Type     ConfigActionType
	Index    int       // Request index for Send actions
	Packet   []byte    // Packet to send for SendRequest action
	Speed    int       // Speed change after send (0 if none)
	Deadline time.Time // Deadline for WaitUntil actions
	Error    error     // Error details for Error action
}

// NewConfigDirector creates a new ConfigDirector for the given Configurator.
// maxRetries specifies the maximum number of retry attempts for timed-out requests.
func NewConfigDirector(cfgtor Configurator, maxRetries int) *ConfigDirector {
	return &ConfigDirector{
		cfgtor:              cfgtor,
		maxRetries:          maxRetries,
		speedChangeRequests: make(map[int]struct{}),
	}
}

// AdvanceTimeTo updates the ConfigDirector's current time and automatically processes timeouts.
// This method should be called by clients to update the director's time reference.
// Time cannot go backwards - this will panic if t is before the current time.
func (cd *ConfigDirector) AdvanceTimeTo(t time.Time) {
	if t.Before(cd.currentTime) {
		panic("ConfigDirector: AdvanceTime moved backwards")
	}
	cd.currentTime = t
	cd.advanceCount++
	cd.processTimeouts()
}

// processTimeouts automatically processes timeouts for requests whose deadlines have passed.
// This is called automatically by AdvanceTimeTo.
func (cd *ConfigDirector) processTimeouts() {
	count, _ := cd.cfgtor.GetRequestCount()
	for i := range count {
		req := cd.cfgtor.Request(i)
		state := req.GetState()
		switch state {
		case ConfigRequestAwaitingResponse, ConfigRequestMaybeComplete, ConfigRequestPausing:
			deadline := req.GetDeadline()
			if !cd.currentTime.Before(deadline) {
				req.SetDeadlinePassed()
			}
		}
	}
}

// ValidPacketReceived should be called when a packet with valid checksum is received.
// It notifies requests that may be able to use this as confirmation of a speed change.
func (cd *ConfigDirector) ValidPacketReceived(t time.Time) {
	// Check speed change requests that are awaiting response
	for i := range cd.speedChangeRequests {
		// Skip if outside active window
		if i < cd.startIndex || i >= cd.endIndex {
			continue
		}
		req := cd.cfgtor.Request(i)
		if req.GetState() != ConfigRequestAwaitingResponse {
			continue
		}
		// Let the request decide if this packet confirms its speed change
		req.MaybeSpeedChangeSucceeded(t)
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
		lastWaitAdvanceCount := -1
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
				case ConfigRequestAwaitingResponse, ConfigRequestMaybeComplete, ConfigRequestPausing:
					// Track deadline for WaitUntil action (timeouts are handled automatically by AdvanceTimeTo)
					deadline := req.GetDeadline()
					if earliestDeadline.IsZero() || deadline.Before(earliestDeadline) {
						earliestDeadline = deadline
					}

				case ConfigRequestMayResend:
					// Speed changes should never be retried
					if req.GetSpeedChangeAfter() != 0 {
						req.SetWontResend()
						break
					}

					// Handle retry logic for non-speed-change requests
					cd.ensureRetriesSize(i + 1)
					cd.retries[i]++
					if cd.retries[i] >= cd.maxRetries {
						req.SetWontResend()
						break
					}
					fallthrough

				case ConfigRequestReadyToSend:
					speedChange := req.GetSpeedChangeAfter()
					if speedChange != 0 {
						cd.speedChangeRequests[i] = struct{}{}
					}
					if !yield(ConfigAction{
						Type:   ConfigActionSendRequest,
						Index:  i,
						Packet: req.GetPacket(),
						Speed:  speedChange,
					}) {
						return
					}
				}
			}

			// If we have any awaiting requests, yield wait action
			if !earliestDeadline.IsZero() {
				// Robustness check: ensure the client reported time after waiting.
				if cd.advanceCount == lastWaitAdvanceCount {
					panic("ConfigDirector API misuse: client must call AdvanceTimeTo() after ConfigActionWaitUntil")
				}
				lastWaitAdvanceCount = cd.advanceCount

				if !yield(ConfigAction{Type: ConfigActionWaitUntil, Deadline: earliestDeadline}) {
					return
				}
			} else if cd.startIndex >= cd.endIndex && cd.startIndex < count {
				// No deadline to wait for AND no actionable requests in window
				// There are still requests but none are actionable
				// This means they're stuck in NotReady state - Configurator bug
				panic("Configurator bug: requests stuck in NotReady state with no way to progress")

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
	if len(cd.retries) >= size {
		return
	}
	newRetries := make([]int, size)
	copy(newRetries, cd.retries)
	cd.retries = newRetries
}
