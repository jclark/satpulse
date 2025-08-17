package unc

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/jclark/satpulse/internal/gpsprot"
	"github.com/jclark/satpulse/internal/nmea"
	"github.com/jclark/satpulse/internal/uncmsg"
)

type ConfigProtocol struct {
	ver *uncmsg.Version // Stored from VERSIONB response for probing
	cfg *Configurator   // Created during Configure() call
}

type Configurator struct {
	phase       configPhase
	ver         *uncmsg.Version
	nativeProps nativeConfigProps
	reqs        []*ConfigRequest
	target      *gpsprot.ConfigTarget
	complete    bool // no more requests will be added to req
	nFinished   int  // number of finished requests
}

type configPhase int

const (
	phaseInit configPhase = iota
	phaseQuery
	phaseFinal
)

type ConfigRequest struct {
	state configRequestState
	cmd   string           // not including CR/LF
	prop  nativeConfigProp // if non-nil, then this means it's a command that updates that prop
	tSent time.Time
	err   error // error in handling this request
}

type configRequestState int

const (
	stateReadyToSendCommand configRequestState = iota // maps to ConfigRequestReadyToSend
	stateReadyToSendQuery                             // maps to ConfigRequestReadyToSend
	stateAwaitingAckAndResponse                       // maps to ConfigRequestAwaitingResponse
	stateAwaitingAck                                  // maps to ConfigRequestAwaitingResponse
	stateAwaitingResponse                             // maps to ConfigRequestAwaitingResponse
	stateMayResendCommand                             // maps to ConfigRequestMayResend
	stateMayResendQuery                               // maps to ConfigRequestMayResend
	stateFailed                                       // maps to ConfigRequestFailed
	stateSucceeded                                    // maps to ConfigRequestSucceeded
)

func NewConfigProtocol() *ConfigProtocol {
	return &ConfigProtocol{}
}

func (cp *ConfigProtocol) NativeMsg(tag gpsprot.Tag, msgID string, msg interface{}, tRead time.Time) error {
	switch mt := msg.(type) {
	case *nmea.Sentence:
		if cp.cfg != nil {
			return cp.cfg.nmeaSentence(mt, tRead)
		}

	case *uncmsg.Mode:
		if cp.cfg != nil {
			return cp.cfg.modeResponse(mt, tRead)
		}
	case *uncmsg.Version:
		cp.ver = mt
	}
	return nil
}

func (cp *ConfigProtocol) ProbePacket() []byte {
	return []byte("VERSIONB\r\n")
}

func (cp *ConfigProtocol) ProbeOK() bool {
	return cp.ver != nil
}

func (cp *ConfigProtocol) Configure(target *gpsprot.ConfigTarget) (*Configurator, error) {
	if cp.ver == nil {
		panic("Configure called without successful ProbeOK()")
	}
	return &Configurator{
		target:      target,
		ver:         cp.ver,
		nativeProps: makeNativeProps(),
		phase:       phaseInit,
	}, nil
}

func (c *Configurator) Request(index int) *ConfigRequest {
	return c.reqs[index]
}

func (c *Configurator) GetRequestCount() (int, bool) {
	return len(c.reqs), c.complete
}

func (c *Configurator) GenerateRequests() error {
	switch c.phase {
	case phaseInit:
		c.generateQueryReqs()
		c.phase = phaseQuery
		return nil
		
	case phaseQuery:
		if !c.allRequestsFinal() {
			return nil // wait for query responses
		}
		
		// All queries are final, generate config requests if none failed
		if !c.anyRequestsFailed() {
			c.generateConfigReqs()
		}
		c.complete = true
		c.phase = phaseFinal
		return nil
		
	case phaseFinal:
		return nil // nothing to do
	}
	
	return nil
}

func (c *Configurator) generateQueryReqs() {
	for _, cmd := range generateQueryCommands(c.target) {
		c.reqs = append(c.reqs, &ConfigRequest{
			cmd:   cmd,
			state: stateReadyToSendQuery,
		})
	}
}

func (c *Configurator) allRequestsFinal() bool {
	for _, req := range c.reqs {
		if !req.state.isFinal() {
			return false
		}
	}
	return true
}

func (c *Configurator) anyRequestsFailed() bool {
	for _, req := range c.reqs {
		if req.state == stateFailed {
			return true
		}
	}
	return false
}

func (c *Configurator) generateConfigReqs() {
	for _, cmd := range c.nativeProps.generateConfigCommands(c.target) {
		c.reqs = append(c.reqs, &ConfigRequest{
			cmd:   cmd,
			state: stateReadyToSendCommand,
		})
	}
}

func (c *Configurator) nmeaSentence(sentence *nmea.Sentence, tRead time.Time) error {
	// Extract payload from the sentence (content between $ and *)
	payload := sentence.Payload

	fields := strings.Split(payload, ",")
	if len(fields) == 0 {
		return nil
	}

	format := fields[0]
	data := fields[1:]
	switch format {
	case "command":
		return c.commandResponse(data, tRead)
	case "CONFIG":
		return c.configQueryResponse(data, tRead)
	}
	return nil
}

func (req *ConfigRequest) Packet() []byte {
	// Precondition: state maps to ConfigRequestReadyToSend, ConfigRequestMayResend, or ConfigRequestFailed
	switch req.state {
	case stateReadyToSendCommand, stateReadyToSendQuery, stateMayResendCommand, stateMayResendQuery, stateFailed:
		// Valid states
	default:
		panic(fmt.Sprintf("Packet called in invalid state: %v", req.state))
	}
	return append([]byte(req.cmd), '\r', '\n')
}

func (req *ConfigRequest) SetSentTime(tSent time.Time) {
	// Precondition: state maps to ConfigRequestReadyToSend or ConfigRequestMayResend
	switch req.state {
	case stateReadyToSendCommand:
		// Commands need ACKs only
		req.state = stateAwaitingAck
	case stateReadyToSendQuery:
		// Queries need both ACK and response
		req.state = stateAwaitingAckAndResponse
	case stateMayResendCommand:
		// Retry command - back to awaiting ACK
		req.state = stateAwaitingAck
	case stateMayResendQuery:
		// Retry query - back to awaiting ACK and response
		req.state = stateAwaitingAckAndResponse
	default:
		panic(fmt.Sprintf("SetSentTime called in invalid state: %v", req.state))
	}
	req.tSent = tSent
}

func (req *ConfigRequest) SetTimedOut() {
	// Precondition: state maps to ConfigRequestAwaitingResponse
	switch req.state {
	case stateAwaitingAck:
		// Command timed out waiting for ACK
		req.state = stateMayResendCommand
	case stateAwaitingAckAndResponse, stateAwaitingResponse:
		// Query timed out waiting for ACK and/or response
		// (stateAwaitingResponse is reached after query gets ACK but still needs response)
		req.state = stateMayResendQuery
	default:
		panic(fmt.Sprintf("SetTimedOut called in invalid state: %v", req.state))
	}
}

func (req *ConfigRequest) SetWontResend() {
	// Precondition: state maps to ConfigRequestMayResend  
	switch req.state {
	case stateMayResendCommand, stateMayResendQuery:
		req.state = stateFailed
		if req.err == nil {
			req.err = fmt.Errorf("request abandoned after timeout")
		}
	default:
		panic(fmt.Sprintf("SetWontResend called in invalid state: %v", req.state))
	}
}

// TODO: Add GetState() method when gpsprot.ConfigRequestState is available
// func (req *ConfigRequest) GetState() gpsprot.ConfigRequestState {
//     // Map internal state to public state
// }

var responseFieldRegexp = regexp.MustCompile(`^response(?:: OK|[: ](.+))$`)

// handle a command response of the form
// `$command,CONFIG,response: OK*XX\r\f`
// fields here will be {"CONFIG", "response: OK"}
func (c *Configurator) commandResponse(fields []string, tRead time.Time) error {
	if len(fields) != 2 {
		return fmt.Errorf("invalid command response format: %s", fields)
	}
	cmd := fields[0]
	response := fields[1]
	matches := responseFieldRegexp.FindStringSubmatch(response)
	if matches == nil {
		return fmt.Errorf("invalid command response: %s", response)
	}
	responseErr := matches[1]
	for i := c.nFinished; i < len(c.reqs); i++ {
		req := c.reqs[i]
		if req.handleAck(cmd, responseErr, tRead) {
			return nil
		}
	}
	return fmt.Errorf("no matching request for command response: %s", cmd)
}

const maxResponseDelay = time.Second * 3 / 2

// handleAck processes an ACK response for this request.
// Returns true if this ACK matches this request, false otherwise.
func (req *ConfigRequest) handleAck(cmd string, responseErr string, tRead time.Time) bool {
	// Check if this ACK is for this request
	if req.cmd != cmd {
		return false
	}
	
	// Check timing - ACK should come after request was sent
	delay := tRead.Sub(req.tSent)
	if delay < 0 || delay > maxResponseDelay {
		return false
	}
	
	// Check if we're in a state that expects an ACK and optimistically set success state
	switch req.state {
	case stateAwaitingAck:
		req.state = stateSucceeded
	case stateAwaitingAckAndResponse:
		req.state = stateAwaitingResponse
	default:
		return false
	}
	
	// This ACK is for us - check if it's an error
	if responseErr != "" {
		req.state = stateFailed
		req.err = fmt.Errorf("command rejected: %s", responseErr)
		return true
	}
	
	// ACK was positive - update our state to reflect the successful command
	if req.prop != nil {
		err := req.prop.updateFromCommand(req.cmd)
		if err != nil {
			panic(fmt.Sprintf("could not parse generated command: %s: %v", req.cmd, err))
		}
	}
	
	return true
}

// configQueryResponse handles a response to query, where the response has the form
// `$CONFIG,PPS,CONFIG PPS ENABLE GPS POSITIVE 100000 1000 0 0*6A`
// fields here with be {"PPS","CONFIG PPS ENABLE GPS POSITIVE 100000 1000 0 0"}
func (c *Configurator) configQueryResponse(fields []string, tRead time.Time) error {
	if len(fields) < 2 {
		return fmt.Errorf("invalid config query response format: %v", fields)
	}

	return c.queryResponse("CONFIG", fields[0], fields[1], tRead)
}

func (c *Configurator) modeResponse(mode *uncmsg.Mode, tRead time.Time) error {
	// Combine Mode and HeadingMode fields as expected by updateFromCommand
	cmd := mode.Mode
	if mode.HeadingMode != "" {
		// Include HEADING2 information if present - use space separator
		cmd = cmd + " " + mode.HeadingMode
	}
	return c.queryResponse("MODE", "MODE", cmd, tRead)
}

func (c *Configurator) queryResponse(query, key, command string, tRead time.Time) error {
	// Find matching request that is awaiting a response
	for _, req := range c.reqs {
		if req.state == stateAwaitingResponse && req.cmd == query {
			// Check timing - response should come after request was sent
			if !tRead.Before(req.tSent) {
				req.state = stateSucceeded
				return c.nativeProps.updateFromQueryResponse(key, command)
			}
		}
	}
	return fmt.Errorf("no matching request for query response: %s", key)
}

func (state configRequestState) isFinal() bool {
	switch state {
	case stateFailed, stateSucceeded:
		return true
	}
	return false
}
