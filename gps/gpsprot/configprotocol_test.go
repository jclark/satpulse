package gpsprot

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"
)

// mockRequest implements ConfigRequest for testing
type mockRequest struct {
	packet       []byte
	speedChange  int
	state        ConfigRequestState
	deadline     time.Time
	readyTime    time.Time
	err          error
	sentTime     time.Time
	responseTime time.Time // For MaybeComplete state
}

func TestConfigSupportFlagsItems(t *testing.T) {
	flags := ConfigSupportSignal |
		ConfigSupportSurveyMsg |
		ConfigSupportFixedPos |
		ConfigSupportRTCMMSM7 |
		ConfigSupportRTCMQZSS |
		ConfigSupportReload
	want := []string{"signal", "surveyMsg", "fixedPos", "rtcmMSM7", "rtcmQZSS", "reload"}
	if got := flags.Items(); !slices.Equal(got, want) {
		t.Errorf("Items() = %v, want %v", got, want)
	}
}

func TestConfigSupportLast(t *testing.T) {
	var highest ConfigSupportFlags
	for _, entry := range configSupportFlagNames {
		if entry.flag&configSupportQualifiers == 0 && entry.flag > highest {
			highest = entry.flag
		}
	}
	if ConfigSupportLast != highest {
		t.Errorf("ConfigSupportLast = %v, want highest declared capability flag %v", ConfigSupportLast, highest)
	}
	if ConfigSupportFull != ConfigSupportLast<<1-1 {
		t.Errorf("ConfigSupportFull = %v, want all capability flags %v", ConfigSupportFull, ConfigSupportLast<<1-1)
	}
	if configSupportQualifiers&ConfigSupportFull != 0 {
		t.Errorf("qualifier flags overlap ConfigSupportFull: %v", configSupportQualifiers&ConfigSupportFull)
	}
}

func TestConfigSupportFlagsString(t *testing.T) {
	flags := ConfigSupportSpeed | ConfigSupportRaw | ConfigSupportRTCMBaseID | ConfigSupportReload
	want := "speed, raw, rtcmBaseID, reload"
	if got := flags.String(); got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}

func TestConfigSupportFlagsMarshalJSON(t *testing.T) {
	tests := []struct {
		flags ConfigSupportFlags
		want  string
	}{
		{0, `[]`},
		{ConfigSupportSignal | ConfigSupportFixedPos | ConfigSupportRaw | ConfigSupportReload, `["signal","fixedPos","raw","reload"]`},
	}
	for _, tt := range tests {
		b, err := json.Marshal(tt.flags)
		if err != nil {
			t.Fatalf("Marshal(%v): %v", tt.flags, err)
		}
		if got := string(b); got != tt.want {
			t.Errorf("Marshal(%v) = %s, want %s", tt.flags, got, tt.want)
		}
	}
}

func (r *mockRequest) GetPacket() []byte {
	switch r.state {
	case ConfigRequestReadyToSend, ConfigRequestMayResend, ConfigRequestFailed:
		return r.packet
	default:
		panic(fmt.Sprintf("GetPacket called in invalid state: %v", r.state))
	}
}

func (r *mockRequest) GetSpeedChangeAfter() int {
	switch r.state {
	case ConfigRequestReadyToSend, ConfigRequestMayResend, ConfigRequestFailed:
		return r.speedChange
	default:
		panic(fmt.Sprintf("GetSpeedChangeAfter called in invalid state: %v", r.state))
	}
}

func (r *mockRequest) GetState() ConfigRequestState {
	return r.state
}

func (r *mockRequest) GetDeadline() time.Time {
	switch r.state {
	case ConfigRequestAwaitingResponse:
		if r.deadline.IsZero() {
			return r.sentTime.Add(time.Second)
		}
		return r.deadline
	case ConfigRequestMaybeComplete:
		if r.responseTime.IsZero() {
			r.responseTime = r.sentTime.Add(10 * time.Millisecond)
		}
		return r.responseTime.Add(50 * time.Millisecond)
	case ConfigRequestPausing:
		// This shouldn't be called for pausing requests, but return a safe value
		return r.readyTime
	default:
		panic(fmt.Sprintf("GetDeadline called in invalid state: %v", r.state))
	}
}

func (r *mockRequest) GetReadyTime() time.Time {
	if r.state != ConfigRequestPausing {
		panic(fmt.Sprintf("GetReadyTime called in invalid state: %v", r.state))
	}
	return r.readyTime
}

func (r *mockRequest) GetError() error {
	if r.state != ConfigRequestFailed {
		panic(fmt.Sprintf("GetError called in invalid state: %v", r.state))
	}
	return r.err
}

func (r *mockRequest) SetSentTime(tSent time.Time) {
	switch r.state {
	case ConfigRequestReadyToSend:
		r.sentTime = tSent
		r.state = ConfigRequestAwaitingResponse
	case ConfigRequestMayResend:
		r.sentTime = tSent
		r.state = ConfigRequestAwaitingResponse
	default:
		panic(fmt.Sprintf("SetSentTime called in invalid state: %v", r.state))
	}
}

func (r *mockRequest) SetDeadlinePassed() {
	switch r.state {
	case ConfigRequestAwaitingResponse:
		r.state = ConfigRequestMayResend
	case ConfigRequestMaybeComplete:
		r.state = ConfigRequestSucceeded
	case ConfigRequestPausing:
		// When pause deadline passes, request becomes ready to send
		r.state = ConfigRequestReadyToSend
	default:
		panic(fmt.Sprintf("SetDeadlinePassed called in invalid state: %v", r.state))
	}
}

func (r *mockRequest) SetWontResend() {
	if r.state != ConfigRequestMayResend {
		panic(fmt.Sprintf("SetWontResend called in invalid state: %v", r.state))
	}
	r.state = ConfigRequestFailed
	if r.err == nil {
		r.err = errors.New("request abandoned after timeout")
	}
}

func (r *mockRequest) MaybeSpeedChangeSucceeded(validPacketTime time.Time) {
	// Mock doesn't implement speed changes
}

// mockConfigurator implements Configurator for testing
type mockConfigurator struct {
	requests      []*mockRequest
	complete      bool
	generateErr   error
	generateCalls int
	props         *ConfigProps
	info          *ReceiverInfo
	support       ConfigSupportFlags
}

func (c *mockConfigurator) GenerateRequests() error {
	c.generateCalls++
	return c.generateErr
}

func (c *mockConfigurator) GetRequestCount() (int, bool) {
	return len(c.requests), c.complete
}

func (c *mockConfigurator) Request(index int) ConfigRequest {
	if index >= len(c.requests) {
		panic(fmt.Sprintf("index %d >= request count %d", index, len(c.requests)))
	}
	return c.requests[index]
}

func (c *mockConfigurator) ConfigProps() *ConfigProps {
	if c.props == nil {
		c.props = &ConfigProps{}
	}
	return c.props
}

func (c *mockConfigurator) ReceiverInfo() *ReceiverInfo {
	if c.info == nil {
		c.info = &ReceiverInfo{}
	}
	return c.info
}

func (c *mockConfigurator) ConfigSupport() ConfigSupportFlags {
	return c.support
}

// Test basic ConfigDirector operation with simple requests
func TestConfigDirectorBasic(t *testing.T) {
	cfg := &mockConfigurator{
		requests: []*mockRequest{
			{packet: []byte("req1"), state: ConfigRequestReadyToSend},
			{packet: []byte("req2"), state: ConfigRequestNotReady},
		},
		complete: true,
	}

	director := NewConfigDirector(cfg, 3)
	var actions []ConfigAction
	testTime := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)

	for action := range director.Actions() {
		actions = append(actions, action)

		// Simulate client behavior
		if action.Type == ConfigActionSendRequest {
			req := cfg.requests[action.Index]
			req.SetSentTime(testTime)
			testTime = testTime.Add(10 * time.Millisecond)
			// Simulate immediate success (no response needed)
			req.state = ConfigRequestSucceeded

			// Make next request ready
			if action.Index+1 < len(cfg.requests) {
				cfg.requests[action.Index+1].state = ConfigRequestReadyToSend
			}
		}
	}

	// Verify we got send actions for both requests
	sendCount := 0
	for _, a := range actions {
		if a.Type == ConfigActionSendRequest {
			sendCount++
		}
	}
	if sendCount != 2 {
		t.Errorf("expected 2 send actions, got %d", sendCount)
	}

	// Verify GenerateRequests was called
	if cfg.generateCalls == 0 {
		t.Error("GenerateRequests was never called")
	}
}

// Test retry logic
func TestConfigDirectorRetry(t *testing.T) {
	cfg := &mockConfigurator{
		requests: []*mockRequest{
			{packet: []byte("req1"), state: ConfigRequestReadyToSend},
		},
		complete: true,
	}

	director := NewConfigDirector(cfg, 3) // Max 3 retries to get 3 total attempts
	var actions []ConfigAction
	sendCount := 0
	testTime := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)

	for action := range director.Actions() {
		actions = append(actions, action)

		switch action.Type {
		case ConfigActionSendRequest:
			sendCount++
			req := cfg.requests[action.Index]
			req.SetSentTime(testTime)
			testTime = testTime.Add(10 * time.Millisecond)

		case ConfigActionWaitUntil:
			// Advance time past deadline to trigger timeout
			director.AdvanceTimeTo(action.Deadline.Add(time.Millisecond))
		}
	}

	// Verify we got the expected number of send actions (1 original + 2 retries = 3 total)
	if sendCount != 3 {
		t.Errorf("expected 3 send actions (1 original + 2 retries), got %d", sendCount)
	}
}

// Test max retry exceeded
func TestConfigDirectorMaxRetryExceeded(t *testing.T) {
	cfg := &mockConfigurator{
		requests: []*mockRequest{
			{packet: []byte("req1"), state: ConfigRequestReadyToSend},
		},
		complete: true,
	}

	director := NewConfigDirector(cfg, 1) // Max 1 retry
	var errorAction *ConfigAction
	testTime := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)

	for action := range director.Actions() {
		switch action.Type {
		case ConfigActionSendRequest:
			req := cfg.requests[action.Index]
			req.SetSentTime(testTime)
			testTime = testTime.Add(10 * time.Millisecond)

		case ConfigActionWaitUntil:
			// Advance time past deadline to trigger timeout
			director.AdvanceTimeTo(action.Deadline.Add(time.Millisecond))

		case ConfigActionError:
			errorAction = &action
		}
	}

	// Verify we got an error after max retries
	if errorAction == nil {
		t.Fatal("expected error action after max retries exceeded")
	}

	// Verify error count
	if director.ErrorCount != 1 {
		t.Errorf("expected ErrorCount=1, got %d", director.ErrorCount)
	}
}

// Test multi-response query handling (MaybeComplete state)
func TestConfigDirectorMultiResponse(t *testing.T) {
	cfg := &mockConfigurator{
		requests: []*mockRequest{
			{packet: []byte("query"), state: ConfigRequestReadyToSend},
		},
		complete: true,
	}

	director := NewConfigDirector(cfg, 3)
	maybeCompleteChecks := 0
	testTime := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)

	for action := range director.Actions() {
		switch action.Type {
		case ConfigActionSendRequest:
			req := cfg.requests[action.Index]
			req.SetSentTime(testTime)
			testTime = testTime.Add(10 * time.Millisecond)
			// Simulate multi-response by transitioning to MaybeComplete
			req.state = ConfigRequestMaybeComplete
			maybeCompleteChecks++

		case ConfigActionWaitUntil:
			// Check if waiting for more responses
			req := cfg.requests[0]
			if req.state == ConfigRequestMaybeComplete {
				maybeCompleteChecks++
			}
			// Advance time to complete the wait
			director.AdvanceTimeTo(action.Deadline.Add(time.Millisecond))
		}
	}

	// Verify we handled MaybeComplete state
	if maybeCompleteChecks < 2 {
		t.Errorf("expected at least 2 MaybeComplete checks, got %d", maybeCompleteChecks)
	}

	// Verify request succeeded
	if cfg.requests[0].state != ConfigRequestSucceeded {
		t.Errorf("expected request to succeed, got state %v", cfg.requests[0].state)
	}
}

func TestConfigDirectorEqualTimeAdvanceAfterWait(t *testing.T) {
	testTime := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	cfg := &mockConfigurator{
		requests: []*mockRequest{
			{
				packet:   []byte("req1"),
				state:    ConfigRequestAwaitingResponse,
				deadline: testTime.Add(time.Second),
			},
		},
		complete: true,
	}

	director := NewConfigDirector(cfg, 3)
	waitCount := 0
	for action := range director.Actions() {
		if action.Type != ConfigActionWaitUntil {
			t.Fatalf("action.Type = %v, want ConfigActionWaitUntil", action.Type)
		}
		waitCount++
		director.AdvanceTimeTo(testTime)
		if waitCount == 2 {
			cfg.requests[0].state = ConfigRequestSucceeded
		}
	}

	if waitCount != 2 {
		t.Fatalf("waitCount = %d, want 2", waitCount)
	}
}

func TestConfigDirectorPanicWithoutAdvanceAfterWait(t *testing.T) {
	testTime := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	cfg := &mockConfigurator{
		requests: []*mockRequest{
			{
				packet:   []byte("req1"),
				state:    ConfigRequestAwaitingResponse,
				deadline: testTime.Add(time.Second),
			},
		},
		complete: true,
	}

	director := NewConfigDirector(cfg, 3)
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("Actions() did not panic without AdvanceTimeTo after WaitUntil")
		}
		const want = "ConfigDirector API misuse: client must call AdvanceTimeTo() after ConfigActionWaitUntil"
		if r != want {
			t.Fatalf("panic = %q, want %q", r, want)
		}
	}()
	director.Actions()(func(action ConfigAction) bool {
		if action.Type != ConfigActionWaitUntil {
			t.Fatalf("action.Type = %v, want ConfigActionWaitUntil", action.Type)
		}
		return true
	})
}

// Test pausing state
func TestConfigDirectorPausing(t *testing.T) {
	testTime := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	pauseTime := testTime.Add(100 * time.Millisecond)
	cfg := &mockConfigurator{
		requests: []*mockRequest{
			{packet: []byte("req1"), state: ConfigRequestReadyToSend},
			{packet: []byte("req2"), state: ConfigRequestNotReady},
		},
		complete: true,
	}

	director := NewConfigDirector(cfg, 3)
	sawPausing := false

	for action := range director.Actions() {
		switch action.Type {
		case ConfigActionSendRequest:
			req := cfg.requests[action.Index]
			req.SetSentTime(testTime)
			testTime = testTime.Add(10 * time.Millisecond)

			if action.Index == 0 {
				// First request needs pause
				req.state = ConfigRequestPausing
				req.readyTime = pauseTime
			} else {
				// Second request succeeds immediately
				req.state = ConfigRequestSucceeded
			}

		case ConfigActionWaitUntil:
			// Check if we're waiting for pause
			req := cfg.requests[0]
			if req.state == ConfigRequestPausing {
				sawPausing = true
				// Advance time to complete pause
				director.AdvanceTimeTo(action.Deadline.Add(time.Millisecond))
				// After pause, mark as succeeded and make next ready
				req.state = ConfigRequestSucceeded
				if len(cfg.requests) > 1 {
					cfg.requests[1].state = ConfigRequestReadyToSend
				}
			} else {
				// Advance time for other waits
				director.AdvanceTimeTo(action.Deadline.Add(time.Millisecond))
			}
		}
	}

	if !sawPausing {
		t.Error("did not see pausing state")
	}
}

// Test window management with multiple actionable requests
func TestConfigDirectorWindow(t *testing.T) {
	cfg := &mockConfigurator{
		requests: []*mockRequest{
			{packet: []byte("req1"), state: ConfigRequestReadyToSend},
			{packet: []byte("req2"), state: ConfigRequestReadyToSend}, // Both ready
			{packet: []byte("req3"), state: ConfigRequestNotReady},
		},
		complete: true,
	}

	director := NewConfigDirector(cfg, 3)
	sendIndices := []int{}
	testTime := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)

	for action := range director.Actions() {
		if action.Type == ConfigActionSendRequest {
			sendIndices = append(sendIndices, action.Index)
			// Mark as sent
			cfg.requests[action.Index].SetSentTime(testTime)
			testTime = testTime.Add(10 * time.Millisecond)
			// Simulate immediate success
			cfg.requests[action.Index].state = ConfigRequestSucceeded

			// Make next request ready if exists
			if action.Index+1 < len(cfg.requests) &&
				cfg.requests[action.Index+1].state == ConfigRequestNotReady {
				cfg.requests[action.Index+1].state = ConfigRequestReadyToSend
			}
		}
	}

	// Verify we processed requests in order
	expected := []int{0, 1, 2}
	if !slices.Equal(sendIndices, expected) {
		t.Errorf("expected send indices %v, got %v", expected, sendIndices)
	}
}

// Test error during GenerateRequests
func TestConfigDirectorGenerateError(t *testing.T) {
	generateErr := errors.New("generation failed")
	cfg := &mockConfigurator{
		generateErr: generateErr,
	}

	director := NewConfigDirector(cfg, 3)
	var errorAction *ConfigAction

	for action := range director.Actions() {
		if action.Type == ConfigActionError {
			errorAction = &action
			break
		}
	}

	if errorAction == nil {
		t.Fatal("expected error action from GenerateRequests failure")
	}

	if errorAction.Error != generateErr {
		t.Errorf("expected error %v, got %v", generateErr, errorAction.Error)
	}

	if director.ErrorCount != 1 {
		t.Errorf("expected ErrorCount=1, got %d", director.ErrorCount)
	}
}

// Test skipped requests
func TestConfigDirectorSkipped(t *testing.T) {
	cfg := &mockConfigurator{
		requests: []*mockRequest{
			{packet: []byte("req1"), state: ConfigRequestFailed, err: errors.New("failed")},
			{packet: []byte("req2"), state: ConfigRequestSkipped},
			{packet: []byte("req3"), state: ConfigRequestReadyToSend},
		},
		complete: true,
	}

	director := NewConfigDirector(cfg, 3)
	var actions []ConfigAction
	actionCount := 0

	for action := range director.Actions() {
		actions = append(actions, action)
		actionCount++

		if action.Type == ConfigActionSendRequest && action.Index == 2 {
			// Third request succeeds
			cfg.requests[2].state = ConfigRequestSucceeded
		}
	}

	// Verify we got error for failed request
	errorCount := 0
	sendCount := 0
	for _, a := range actions {
		if a.Type == ConfigActionError {
			errorCount++
		}
		if a.Type == ConfigActionSendRequest {
			sendCount++
		}
	}

	if errorCount != 1 {
		t.Errorf("expected 1 error action for failed request, got %d", errorCount)
	}

	// Only the third request should be sent (first failed, second skipped)
	if sendCount != 1 {
		t.Errorf("expected 1 send action, got %d", sendCount)
	}
}

// Test empty configuration (no requests)
func TestConfigDirectorEmpty(t *testing.T) {
	cfg := &mockConfigurator{
		requests: []*mockRequest{},
		complete: true,
	}

	director := NewConfigDirector(cfg, 3)
	actionCount := 0

	for range director.Actions() {
		actionCount++
		if actionCount > 10 {
			t.Fatal("too many actions for empty configuration")
		}
	}

	if actionCount != 0 {
		t.Errorf("expected 0 actions for empty configuration, got %d", actionCount)
	}
}

// Test speed change handling
func TestConfigDirectorSpeedChange(t *testing.T) {
	cfg := &mockConfigurator{
		requests: []*mockRequest{
			{packet: []byte("baud"), state: ConfigRequestReadyToSend, speedChange: 115200},
		},
		complete: true,
	}

	director := NewConfigDirector(cfg, 3)
	var sendAction *ConfigAction

	for action := range director.Actions() {
		if action.Type == ConfigActionSendRequest {
			sendAction = &action
			// Mark as succeeded
			cfg.requests[0].state = ConfigRequestSucceeded
		}
	}

	if sendAction == nil {
		t.Fatal("expected send action")
	}

	if sendAction.Speed != 115200 {
		t.Errorf("expected speed change to 115200, got %d", sendAction.Speed)
	}
}
