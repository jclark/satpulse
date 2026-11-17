package unc

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/internal/nmea"
	"github.com/jclark/satpulse/gps/lib/novmsg"
	"github.com/jclark/satpulse/gps/lib/uncmsg"
)

const Vendor = "Unicore"

// configSupport is everything except the capabilities the Nebulas IV
// firmware does not offer: the survey/fixed-position accuracy variants
// and survey message output. The Nebulas IV does change baud rate and
// can identify its active port, so unlike CASIC it keeps speed and port.
// Declaring it as full minus these gains future universally supported
// flags automatically.
const configSupport gpsprot.ConfigSupportFlags = gpsprot.ConfigSupportFull &^
	(gpsprot.ConfigSupportSurveyAcc | gpsprot.ConfigSupportSurveyMsg |
		gpsprot.ConfigSupportFixedPosAcc)

type ConfigProtocol struct {
	ver *uncmsg.Version // Stored from VERSIONB response for probing
	cfg *Configurator   // Created during Configure() call
}

type Configurator struct {
	phase       configPhase
	ver         *uncmsg.Version
	nativeProps nativeConfigProps
	reqs        []*ConfigRequest
	speedReq    *ConfigRequest // the speed change request, if the target asks for one
	target      *gpsprot.ConfigTarget
	complete    bool // no more requests will be added to req
	nFinished   int  // all requests with index < nFinished are in final state
}

type configPhase int

const (
	phaseInit   configPhase = iota
	phaseQuery              // sending state queries
	phaseConfig             // sending config commands; a speed change comes after these finish
	phaseSpeed              // speed change outstanding; its repeat is generated when the repeat delay passes
	phaseFinal
)

type ConfigRequest struct {
	state configRequestState
	cmd   string           // not including CR/LF
	prop  nativeConfigProp // if non-nil, then this means it's a command that updates that prop
	speed int              // new baud rate to switch to after sending (0 if not a speed change)
	tBase time.Time        // base time for calculating response deadline (sent time or last response time)
	err   error            // error in handling this request
}

type configRequestState int

const (
	stateReadyToSendCommand      configRequestState = iota // maps to ConfigRequestReadyToSend
	stateReadyToSendQuery                                  // maps to ConfigRequestReadyToSend
	stateReadyToSendRepeat                                 // maps to ConfigRequestReadyToSend
	stateAwaitingAckAndResponse                            // maps to ConfigRequestAwaitingResponse
	stateAwaitingAckBeforeRepeat                           // maps to ConfigRequestAwaitingResponse
	stateAwaitingAck                                       // maps to ConfigRequestAwaitingResponse
	stateAwaitingResponse                                  // maps to ConfigRequestAwaitingResponse
	stateMaybeComplete                                     // maps to ConfigRequestMaybeComplete
	stateRepeatSent                                        // maps to ConfigRequestMaybeComplete
	stateMayResendCommand                                  // maps to ConfigRequestMayResend
	stateMayResendQuery                                    // maps to ConfigRequestMayResend
	stateFailed                                            // maps to ConfigRequestFailed
	stateSucceeded                                         // maps to ConfigRequestSucceeded
)

// Compile-time interface compliance checks
var _ gpsprot.ConfigProtocol = (*ConfigProtocol)(nil)
var _ gpsprot.Configurator = (*Configurator)(nil)
var _ gpsprot.ConfigRequest = (*ConfigRequest)(nil)

func NewConfigProtocol() *ConfigProtocol {
	return &ConfigProtocol{}
}

func (cp *ConfigProtocol) NativeMsg(tag gpsprot.Tag, msgID string, msg any, tRead time.Time) error {
	switch mt := msg.(type) {
	case *nmea.Sentence:
		if cp.cfg != nil {
			return cp.cfg.nmeaSentence(mt, tRead)
		}

	case *uncmsg.Msg:
		// Extract the message body and handle based on its type
		switch body := mt.Body.(type) {
		case *uncmsg.Mode:
			if cp.cfg != nil {
				return cp.cfg.modeResponse(body, tRead)
			}
		case *uncmsg.Version:
			cp.ver = body
		}

	case *novmsg.AbbrevAsciiLine:
		if cp.cfg != nil {
			return cp.cfg.abbrevAsciiResponse(mt, tRead)
		}
	}
	return nil
}

// ReceiverInfo returns static information about the GPS receiver
func (c *Configurator) ReceiverInfo() *gpsprot.ReceiverInfo {
	buildNum, _ := c.ver.BuildNumber()
	return &gpsprot.ReceiverInfo{
		Vendor:         Vendor,
		Firmware:       fmt.Sprintf("Build%d", buildNum),
		Hardware:       c.ver.Type.String(),
		SupportedGNSS:  0, // Will be populated from CONFIG query responses
		VendorSpecific: c.ver,
	}
}

// ConfigSupport returns configuration support for this implementation.
func (c *Configurator) ConfigSupport() gpsprot.ConfigSupportFlags {
	return configSupport
}

// ConfigProps returns the current configuration of the GPS receiver.
func (c *Configurator) ConfigProps() *gpsprot.ConfigProps {
	props := &gpsprot.ConfigProps{}
	c.nativeProps.convertToProps(props)
	return props
}

func (cp *ConfigProtocol) ProbePackets() ([][]byte, time.Duration) {
	return [][]byte{[]byte("VERSIONB\r\n")}, 0
}

func (cp *ConfigProtocol) ProbeOK() bool {
	return cp.ver != nil
}

func (cp *ConfigProtocol) Configure(target *gpsprot.ConfigTarget) (gpsprot.Configurator, error) {
	if cp.ver == nil {
		panic("Configure called without successful ProbeOK()")
	}
	cp.cfg = &Configurator{
		target:      target,
		ver:         cp.ver,
		nativeProps: makeNativeProps(),
		phase:       phaseInit,
	}
	return cp.cfg, nil
}

func (c *Configurator) Request(index int) gpsprot.ConfigRequest {
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
		// Scan through requests starting from nFinished
		anyFailed := false
		for i := c.nFinished; i < len(c.reqs); i++ {
			req := c.reqs[i]
			if !req.state.isFinal() {
				// Found a non-final request, stop here
				c.nFinished = i
				return nil
			}
			if req.state == stateFailed {
				anyFailed = true
			}
		}

		// All requests are final
		c.nFinished = len(c.reqs)

		// Generate config requests only if no query failed
		if anyFailed {
			c.complete = true
			c.phase = phaseFinal
			return nil
		}
		c.generateConfigReqs()
		c.phase = phaseConfig
		return nil

	case phaseConfig:
		// Wait for the config commands to finish, then generate the speed
		// change: it must be sent on its own, so that no other commands or
		// responses are in transit when the host switches speed.
		for i := c.nFinished; i < len(c.reqs); i++ {
			if !c.reqs[i].state.isFinal() {
				c.nFinished = i
				return nil
			}
		}
		c.nFinished = len(c.reqs)
		if c.generateSpeedReq() {
			c.phase = phaseSpeed
		} else {
			c.complete = true
			c.phase = phaseFinal
		}
		return nil

	case phaseSpeed:
		switch c.speedReq.state {
		case stateReadyToSendCommand, stateAwaitingAckBeforeRepeat:
			// not yet sent, or still within the repeat delay
			return nil
		case stateAwaitingAck:
			// The repeat delay passed without confirmation. Send the same
			// command again: the receiver is at the new speed by now, so
			// re-applying it is a no-op whose ACK confirms the speed change
			// request (acknowledgments are matched in request order). The
			// repeat is a plain write, not a speed change, and expects no
			// response of its own.
			c.reqs = append(c.reqs, &ConfigRequest{
				cmd:   c.speedReq.cmd,
				state: stateReadyToSendRepeat,
			})
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

func (c *Configurator) generateConfigReqs() {
	for _, nativeCmd := range c.nativeProps.generateTargetCommands(c.target) {
		c.reqs = append(c.reqs, &ConfigRequest{
			cmd:   nativeCmd.cmd,
			prop:  c.nativeProps.getProp(nativeCmd.key),
			state: stateReadyToSendCommand,
		})
	}
}

// generateSpeedReq generates the baud rate change request when the target
// asks for a speed different from the current one, and reports whether it
// did so. The receiver's ACK may be garbled by the speed switch, so if
// nothing confirms the change within speedChangeRepeatDelay the request
// extends its deadline and repeats the command (see phaseSpeed).
func (c *Configurator) generateSpeedReq() bool {
	port := c.nativeProps.port.name
	i := comIndex(port)
	if i < 0 {
		return false
	}
	speed, ok := c.target.Props.GetBaudRate()
	if !ok || speed == 0 || speed == c.nativeProps.com.speeds[i] {
		return false
	}
	c.speedReq = &ConfigRequest{
		cmd:   fmt.Sprintf("CONFIG %s %d", port, speed),
		prop:  &c.nativeProps.com,
		speed: int(speed),
		state: stateReadyToSendCommand,
	}
	c.reqs = append(c.reqs, c.speedReq)
	return true
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

// invalidStatePanic creates a consistent panic message for invalid state
func (req *ConfigRequest) invalidStatePanic(method string) string {
	return fmt.Sprintf("%s called when state is %v", method, req.GetState())
}

func (req *ConfigRequest) GetPacket() []byte {
	// Precondition: state maps to ConfigRequestReadyToSend, ConfigRequestMayResend, or ConfigRequestFailed
	switch req.state {
	case stateReadyToSendCommand, stateReadyToSendQuery, stateReadyToSendRepeat, stateMayResendCommand, stateMayResendQuery, stateFailed:
		// Valid states
	default:
		panic(req.invalidStatePanic("GetPacket"))
	}
	return append([]byte(req.cmd), '\r', '\n')
}

// GetSpeedChangeAfter returns the new baud rate to configure after sending this request.
func (req *ConfigRequest) GetSpeedChangeAfter() int {
	return req.speed
}

func (req *ConfigRequest) GetState() gpsprot.ConfigRequestState {
	// Map internal state to public ConfigRequestState
	switch req.state {
	case stateReadyToSendCommand, stateReadyToSendQuery, stateReadyToSendRepeat:
		return gpsprot.ConfigRequestReadyToSend
	case stateAwaitingAckAndResponse, stateAwaitingAck, stateAwaitingResponse, stateAwaitingAckBeforeRepeat:
		return gpsprot.ConfigRequestAwaitingResponse
	case stateMaybeComplete, stateRepeatSent:
		return gpsprot.ConfigRequestMaybeComplete
	case stateMayResendCommand, stateMayResendQuery:
		return gpsprot.ConfigRequestMayResend
	case stateFailed:
		return gpsprot.ConfigRequestFailed
	case stateSucceeded:
		return gpsprot.ConfigRequestSucceeded
	default:
		panic(fmt.Sprintf("unexpected internal state: %v", req.state))
	}
}

func (req *ConfigRequest) GetReadyTime() time.Time {
	// Unicore doesn't use pausing
	panic(req.invalidStatePanic("GetReadyTime"))
}

func (req *ConfigRequest) GetError() error {
	if req.state != stateFailed {
		panic(req.invalidStatePanic("GetError"))
	}
	return req.err
}

func (req *ConfigRequest) SetSentTime(tSent time.Time) {
	// Precondition: state maps to ConfigRequestReadyToSend or ConfigRequestMayResend
	switch req.state {
	case stateReadyToSendCommand:
		if req.speed != 0 {
			// Speed change: the ACK may be garbled by the switch, so the
			// first deadline triggers a repeat of the command rather than
			// a timeout (see phaseSpeed).
			req.state = stateAwaitingAckBeforeRepeat
		} else {
			// Commands need ACKs only
			req.state = stateAwaitingAck
		}
	case stateReadyToSendQuery:
		// Queries need both ACK and response
		req.state = stateAwaitingAckAndResponse
	case stateReadyToSendRepeat:
		// The repeat expects no response of its own, but can absorb the ACK
		// it elicits when the original speed change request no longer needs it
		req.state = stateRepeatSent
	case stateMayResendCommand:
		// Retry command - back to awaiting ACK
		req.state = stateAwaitingAck
	case stateMayResendQuery:
		// Retry query - back to awaiting ACK and response
		req.state = stateAwaitingAckAndResponse
	default:
		panic(req.invalidStatePanic("SetSentTime"))
	}
	req.tBase = tSent
}

func (req *ConfigRequest) SetDeadlinePassed() {
	switch req.state {
	case stateAwaitingAck:
		// Command timed out waiting for ACK
		req.state = stateMayResendCommand
	case stateAwaitingAckBeforeRepeat:
		// The repeat delay passed without confirmation. Keep awaiting with the
		// normal deadline (tBase is unchanged, so it is relative to the original
		// send); phaseSpeed sees this state change and generates the repeat.
		req.state = stateAwaitingAck
	case stateAwaitingAckAndResponse, stateAwaitingResponse:
		// Query timed out waiting for ACK and/or response
		req.state = stateMayResendQuery
	case stateMaybeComplete:
		// Idle period over - consider complete
		req.state = stateSucceeded
	case stateRepeatSent:
		// Absorb window over - the repeat is done regardless
		req.state = stateSucceeded
	default:
		panic(req.invalidStatePanic("SetDeadlinePassed"))
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
		panic(req.invalidStatePanic("SetWontResend"))
	}
}

// speedChangeDelay is how long after sending a speed change command until we
// can accept a valid packet as confirmation that the speed change succeeded.
// This accounts for the time it takes the receiver to process and apply the
// speed change.
const speedChangeDelay = 400 * time.Millisecond

// MaybeSpeedChangeSucceeded checks if a valid packet can confirm a speed change.
// The ACK for a speed change on the current port arrives around the moment the
// receiver switches speed, so it may be garbled; any valid packet received
// sufficiently long after sending confirms the change instead.
func (req *ConfigRequest) MaybeSpeedChangeSucceeded(validPacketTime time.Time) {
	if req.speed == 0 || (req.state != stateAwaitingAck && req.state != stateAwaitingAckBeforeRepeat) {
		return
	}
	if validPacketTime.Sub(req.tBase) > speedChangeDelay {
		req.state = stateSucceeded
		req.updateProp()
	}
}

func (req *ConfigRequest) GetDeadline() time.Time {
	switch req.state {
	case stateAwaitingAck, stateAwaitingAckAndResponse, stateAwaitingResponse:
		return req.tBase.Add(maxResponseDelay)
	case stateAwaitingAckBeforeRepeat:
		return req.tBase.Add(speedChangeRepeatDelay)
	case stateMaybeComplete:
		return req.tBase.Add(idlePeriod)
	case stateRepeatSent:
		return req.tBase.Add(speedChangeRepeatAckDelay)
	default:
		panic(req.invalidStatePanic("GetDeadline"))
	}
}

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
			// If request at index nFinished just became final, advance by one
			if i == c.nFinished && req.state.isFinal() {
				c.nFinished++
			}
			return nil
		}
	}
	return fmt.Errorf("no matching request for command response: %s", cmd)
}

const (
	maxResponseDelay = time.Second * 3 / 2
	idlePeriod       = 50 * time.Millisecond
	// speedChangeRepeatDelay is how long after sending a speed change to wait
	// for its ACK before repeating the command. By then the receiver has
	// applied the new speed, so the repeat is received intact.
	speedChangeRepeatDelay = 100 * time.Millisecond
	// speedChangeRepeatAckDelay is how long the repeat can absorb an ACK
	// after being sent. The repeat never fails: an unconfirmed speed change
	// is reported through the original request's own deadline.
	speedChangeRepeatAckDelay = 300 * time.Millisecond
)

// handleAck processes an ACK response for this request.
// Returns true if this ACK matches this request, false otherwise.
func (req *ConfigRequest) handleAck(cmd string, responseErr string, tRead time.Time) bool {
	// Check if this ACK is for this request
	if req.cmd != cmd {
		return false
	}

	// Check timing - ACK should come after request was sent
	delay := tRead.Sub(req.tBase)
	if delay < 0 || delay > maxResponseDelay {
		return false
	}

	// Check if we're in a state that can take an ACK and optimistically set success state
	switch req.state {
	case stateAwaitingAck, stateAwaitingAckBeforeRepeat, stateRepeatSent:
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
	req.updateProp()

	return true
}

// updateProp updates the tracked native state to reflect successful
// execution of the request's command.
func (req *ConfigRequest) updateProp() {
	if req.prop != nil {
		err := req.prop.updateFromCommand(req.cmd)
		if err != nil {
			panic(fmt.Sprintf("could not parse generated command: %s: %v", req.cmd, err))
		}
	}
}

// configQueryResponse handles a response to a query.
// The receiver responds to both CONFIG and MASK queries using multiple $CONFIG sentences.
// A CONFIG query produces one sentence per property, e.g. $CONFIG,PPS,... and $CONFIG,SIGNALGROUP,...
// A MASK query produces one sentence per mask, e.g. $CONFIG,MASK,... and $CONFIG,MASK RTK,...
// The fields parameter contains the comma-separated fields after "$CONFIG,":
// fields[0] is the key (e.g. "PPS", "MASK", "MASK RTK") and fields[1] is the command.
func (c *Configurator) configQueryResponse(fields []string, tRead time.Time) error {
	if len(fields) < 2 {
		return fmt.Errorf("invalid config query response format: %v", fields)
	}
	return c.queryResponse(queryResponseKey(fields[0]), fields[0], strings.Join(fields[1:], ","), tRead)
}

// queryResponseKey returns the query that a $CONFIG response belongs to,
// based on the key field. Keys starting with "MASK" belong to the MASK query;
// everything else belongs to the CONFIG query.
func queryResponseKey(key string) string {
	k, _, _ := strings.Cut(key, " ")
	if k == "MASK" {
		return "MASK"
	}
	return "CONFIG"
}

func (c *Configurator) modeResponse(mode *uncmsg.Mode, tRead time.Time) error {
	return c.queryResponse("MODE", "MODE", mode.Command(), tRead)
}

// abbrevAsciiResponse handles a NovAtel abbreviated ASCII (NOVAA) line. Only the
// LOGLIST header line is significant: its first field is the port the
// receiver is currently communicating on. The count and log entry lines that
// trail in after it are indented (empty Name) and ignored, as is any other
// abbreviated ASCII output.
func (c *Configurator) abbrevAsciiResponse(ab *novmsg.AbbrevAsciiLine, tRead time.Time) error {
	if ab.Name != "LOGLIST" || len(ab.Fields) == 0 {
		return nil
	}
	return c.queryResponse("LOGLIST", idPropPort, ab.Fields[0], tRead)
}

func (c *Configurator) queryResponse(query, key, command string, tRead time.Time) error {
	// Find the matching request and determine its index
	var reqIndex int = -1
	for i := c.nFinished; i < len(c.reqs); i++ {
		req := c.reqs[i]
		if req.cmd == query {
			switch req.state {
			case stateAwaitingResponse:
				// First response to this query - check it's within valid window
				delay := tRead.Sub(req.tBase)
				if delay < 0 || delay > maxResponseDelay {
					break
				}
				reqIndex = i

				// Complete any EARLIER MaybeComplete requests
				// (their response burst is definitely done since we got response to later query)
				for j := c.nFinished; j < i; j++ {
					if c.reqs[j].state == stateMaybeComplete {
						c.reqs[j].state = stateSucceeded
					}
				}

				if query == "CONFIG" || query == "MASK" {
					// may have multiple responses
					req.state = stateMaybeComplete
					req.tBase = tRead
				} else {
					// MODE and LOGLIST queries - single significant response
					req.state = stateSucceeded
				}
			case stateMaybeComplete:
				// Additional response for multi-response query
				reqIndex = i
				// Check if this is within the idle period
				if tRead.After(req.tBase.Add(idlePeriod)) {
					// This should not happen - response outside idle period
					return fmt.Errorf("unexpected late response to %s query (%.0fms after last response)",
						query, tRead.Sub(req.tBase).Seconds()*1000)
				}
				req.tBase = tRead
			}
			if reqIndex >= 0 {
				break
			}
		}
	}
	if reqIndex < 0 {
		return fmt.Errorf("no matching request for query response: %s", key)
	}
	return c.nativeProps.updateFromQueryResponse(key, command)
}

func (state configRequestState) isFinal() bool {
	switch state {
	case stateFailed, stateSucceeded:
		return true
	}
	return false
}
