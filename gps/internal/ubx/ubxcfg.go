package ubx

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/ubxbin"
	ucv "github.com/jclark/satpulse/gps/lib/ubxcfgval"
)

// configRequestState is internal counterpart of gpsprot.ConfigRequestState
type configRequestState int

const (
	stateNotReady               configRequestState = iota // Maps to ConfigRequestNotReady
	stateReadyToSend                                      // Maps to ConfigRequestReadyToSend
	stateAwaitingAck                                      // Maps to ConfigRequestAwaitingResponse
	stateAwaitingResponse                                 // Maps to ConfigRequestAwaitingResponse
	stateAwaitingAckAndResponse                           // Maps to ConfigRequestAwaitingResponse
	statePausing                                          // Maps to ConfigRequestPausing
	stateMayResend                                        // Maps to ConfigRequestMayResend
	stateFailed                                           // Maps to ConfigRequestFailed
	stateSucceeded                                        // Maps to ConfigRequestSucceeded
	stateSkipped                                          // Maps to ConfigRequestSkipped
)

// requestOps defines operations available for different request types
type requestOps interface {
	Packet() []byte
	ChangeSpeed() int
	Pause() time.Duration
	Ackable() bool
	AwaitingResponse(sentSeq uint64) bool
	Done()
	ID() string
}

// configRequest is the UBX implementation of gpsprot.ConfigRequest
type configRequest struct {
	ops            requestOps         // Request-specific operations
	cfg            *Configurator      // Back-reference for event sequence lookup
	state          configRequestState // Internal state for ACK/response tracking
	sentTime       time.Time          // When the request was sent
	sentSeq        uint64             // Event sequence visible when the request was sent
	pauseStartTime time.Time          // When the pause period started
	err            error              // Error details for failed requests
}

type Configurator struct {
	ver         *Version // never nil
	tRead       map[ubxbin.MsgID]time.Time
	msgSeq      map[ubxbin.MsgID]uint64
	eventSeq    uint64
	raw         RawConfig
	origPrt     *ubxbin.CfgPrt
	portID      *ubxbin.PortID
	monGNSS     *monGNSS
	planSignals gpsprot.SignalSet // signals for active signal plan from MON-GNSS v1
	steps       []func(*Configurator) error
	stepIndex   int
	reqs        []*configRequest
	nextIndex   int                   // Index of first request that is still in stateNotReady
	complete    bool                  // True when no more requests can be added to reqs
	target      *gpsprot.ConfigTarget // never nil
	survey      bool                  // start a survey

}

// monGNSS records information from UBX-MON-GNSS
// Since enabledGNSS can be invalidated by changes to the GNSS configuration,
// we store the gnssChangeCount at the time monGNSS was created.
// enabledGNSS is only valid if gnssChangeCount in monGNSS is the same as raw.gnssChangeCount.
type monGNSS struct {
	maxSimultaneousMajorGNSS int
	supportedGNSS            gpsprot.GNSSSet
	enabledGNSS              gpsprot.GNSSSet
	gnssChangeCount          int
}

type RawConfig struct {
	CfgOld
	CfgVals             // access with valsPtr() so it gets lazily initialized
	gnssChangeCount int // incremented each time GNSS configuration changes
}

var legacyConfigSteps = []func(*Configurator) error{
	(*Configurator).pollPrt,
	(*Configurator).setMsg1,  // do this ASAP, because responses can be slow (at least on 8th gen) when NMEA is enabled
	(*Configurator).pollGNSS, // need this to know which will be time GNSS, which we need for enabling the time message
	(*Configurator).pollMonGNSS,
	(*Configurator).setGNSS, // do this early because we may need to know enabled GNSS to deduce time GNSS
	(*Configurator).pollTp5,
	(*Configurator).setTp5,
	(*Configurator).pollTmode,
	(*Configurator).setTmode,
	(*Configurator).pollRate,
	(*Configurator).pollNav5,
	(*Configurator).setNav5,
	(*Configurator).setMsg2,
	(*Configurator).setRate, // setRate must come after all the messages have been enabled, so it can tell if rate needs setting
	(*Configurator).setBaudRate,
	(*Configurator).saveMinimal,
	(*Configurator).reloadCfg,
	(*Configurator).setCfg,
	(*Configurator).reset,
}

var newConfigSteps = []func(*Configurator) error{
	(*Configurator).pollMonComms,
	(*Configurator).pollPrt,
	(*Configurator).valPollMonGNSS,
	(*Configurator).valGetSignals,
	// XXX we have to do this early at the moment, because we may need to deduce what GNSS is time GNSS
	(*Configurator).valSetSignals,
	(*Configurator).valGetNMA,
	(*Configurator).valSetNMA,
	(*Configurator).valGet,
	(*Configurator).valSet,
	(*Configurator).timeAssist,
	(*Configurator).osnmaAssist,
	(*Configurator).valBaudRate,
	(*Configurator).reloadCfg,
	(*Configurator).setCfg,
	(*Configurator).reset,
}

var _ gpsprot.Configurator = (*Configurator)(nil)

// GetPacket returns the packet bytes for this request.
func (cr *configRequest) GetPacket() []byte {
	switch cr.state {
	case stateReadyToSend, stateMayResend, stateFailed:
		return cr.ops.Packet()
	default:
		panic(fmt.Sprintf("GetPacket called in wrong state: %v", cr.GetState()))
	}
}

// GetSpeedChangeAfter returns the new baud rate to configure after sending.
func (cr *configRequest) GetSpeedChangeAfter() int {
	switch cr.state {
	case stateReadyToSend, stateMayResend, stateFailed:
		return cr.ops.ChangeSpeed()
	default:
		panic(fmt.Sprintf("GetSpeedChangeAfter called in wrong state: %v", cr.GetState()))
	}
}

// GetState returns the current state of this request.
func (cr *configRequest) GetState() gpsprot.ConfigRequestState {
	switch cr.state {
	case stateNotReady:
		return gpsprot.ConfigRequestNotReady
	case stateReadyToSend:
		return gpsprot.ConfigRequestReadyToSend
	case stateAwaitingAck, stateAwaitingResponse, stateAwaitingAckAndResponse:
		return gpsprot.ConfigRequestAwaitingResponse
	case statePausing:
		return gpsprot.ConfigRequestPausing
	case stateMayResend:
		return gpsprot.ConfigRequestMayResend
	case stateFailed:
		return gpsprot.ConfigRequestFailed
	case stateSucceeded:
		return gpsprot.ConfigRequestSucceeded
	case stateSkipped:
		return gpsprot.ConfigRequestSkipped
	default:
		panic(fmt.Sprintf("unknown internal state: %v", cr.state))
	}
}

// GetDeadline returns the absolute time by which response packets are expected.
func (cr *configRequest) GetDeadline() time.Time {
	switch cr.state {
	case stateAwaitingAck, stateAwaitingResponse, stateAwaitingAckAndResponse:
		// 1500ms timeout matching original gpscfg.go behavior
		return cr.sentTime.Add(1500 * time.Millisecond)
	case statePausing:
		// Return pause completion deadline
		return cr.pauseStartTime.Add(cr.ops.Pause())
	default:
		panic(fmt.Sprintf("GetDeadline called in wrong state: %v", cr.GetState()))
	}
}

// GetError returns the error details for a failed request.
func (cr *configRequest) GetError() error {
	if cr.state != stateFailed {
		panic(fmt.Sprintf("GetError called in wrong state: %v", cr.GetState()))
	}
	return cr.err
}

// SetSentTime records when the request packet was transmitted.
func (cr *configRequest) SetSentTime(tSent time.Time) {
	switch cr.state {
	case stateReadyToSend, stateMayResend:
		// ok
	default:
		panic(fmt.Sprintf("SetSentTime called in wrong state: %v", cr.GetState()))
	}

	cr.sentTime = tSent
	cr.sentSeq = cr.cfg.eventSeq

	// Determine initial awaiting state based on what this request needs
	needsAck := cr.ops.Ackable()
	needsResponse := cr.ops.AwaitingResponse(cr.sentSeq)

	if needsAck && needsResponse {
		cr.state = stateAwaitingAckAndResponse
	} else if needsAck {
		cr.state = stateAwaitingAck
	} else if needsResponse {
		cr.state = stateAwaitingResponse
	} else {
		// Neither ACK nor response needed - immediately succeed
		cr.state = stateSucceeded
		cr.ops.Done()
	}
}

// SetDeadlinePassed notifies that the response deadline has passed.
func (cr *configRequest) SetDeadlinePassed() {
	switch cr.state {
	case stateAwaitingAck, stateAwaitingResponse, stateAwaitingAckAndResponse:
		cr.state = stateMayResend
	case statePausing:
		// Pause duration elapsed - mark as succeeded
		cr.state = stateSucceeded
		cr.ops.Done()
	default:
		panic(fmt.Sprintf("SetDeadlinePassed called in wrong state: %v", cr.GetState()))
	}
}

// SetWontResend marks a request as permanently failed.
func (cr *configRequest) SetWontResend() {
	if cr.state != stateMayResend {
		panic(fmt.Sprintf("SetWontResend called in wrong state: %v", cr.GetState()))
	}
	cr.state = stateFailed
	if cr.err == nil {
		cr.err = fmt.Errorf("no response to request")
	}
}

// MaybeSpeedChangeSucceeded checks if a valid packet can confirm a speed change.
func (cr *configRequest) MaybeSpeedChangeSucceeded(validPacketTime time.Time) {
	// Only applicable if we're awaiting ACK for a speed change
	if cr.state != stateAwaitingAck || cr.ops.ChangeSpeed() == 0 {
		return
	}

	// Check if enough time has passed since sending for the packet to be post-speed-change
	if validPacketTime.Sub(cr.sentTime) > speedChangeDelay {
		// This packet confirms the speed change succeeded
		cr.state = stateSucceeded
		cr.ops.Done()
	}
}

// checkComplete determines if request is fully complete
func (cr *configRequest) checkComplete(ackTime time.Time) {
	switch cr.state {
	case stateAwaitingAck:
		// Got the ACK we were waiting for
		if cr.ops.Pause() > 0 {
			cr.state = statePausing
			cr.pauseStartTime = ackTime
		} else {
			cr.state = stateSucceeded
			cr.ops.Done()
		}
	case stateAwaitingResponse:
		// Check if we have the response
		if !cr.ops.AwaitingResponse(cr.sentSeq) {
			if cr.ops.Pause() > 0 {
				cr.state = statePausing
				cr.pauseStartTime = ackTime
			} else {
				cr.state = stateSucceeded
				cr.ops.Done()
			}
		}
	case stateAwaitingAckAndResponse:
		// If the response arrived, wait for the ACK before completing.
		// Do not recursively call checkComplete here: the ACK path in
		// processAckNak is responsible for advancing stateAwaitingAck.
		if !cr.ops.AwaitingResponse(cr.sentSeq) {
			cr.state = stateAwaitingAck
		}
	}
}

func newConfigurator(target *gpsprot.ConfigTarget, ver *Version) *Configurator {
	steps := legacyConfigSteps
	if ver.protVerGreater(23, 1) {
		steps = newConfigSteps
	}
	return &Configurator{
		ver:    ver,
		target: target,
		steps:  steps,
		tRead:  make(map[ubxbin.MsgID]time.Time),
		msgSeq: make(map[ubxbin.MsgID]uint64),
	}
}

func (c *Configurator) nextEventSeq() uint64 {
	c.eventSeq++
	return c.eventSeq
}

func (c *Configurator) noteMsg(mid ubxbin.MsgID, t time.Time) {
	seq := c.nextEventSeq()
	c.tRead[mid] = t
	c.msgSeq[mid] = seq
}

func (c *Configurator) ConfigProps() *gpsprot.ConfigProps {
	cp := c.raw.Config(c.ver, c.portID)
	if cp != nil && c.target.Get&gpsprot.PropIDPort != 0 {
		if port, ok := c.valPort(); ok {
			if name, ok := portName(port); ok {
				cp.SetPort(name)
			}
		}
	}
	return cp
}

// portName maps a u-blox port identifier to the user-facing port
// name. Returns false for unrecognised values; the result then leaves
// PropIDPort unset rather than emitting a synthetic name.
func portName(p ucv.Port) (string, bool) {
	switch p {
	case ucv.I2C:
		return "I2C", true
	case ucv.UART1:
		return "UART1", true
	case ucv.UART2:
		return "UART2", true
	case ucv.USB:
		return "USB", true
	case ucv.SPI:
		return "SPI", true
	}
	return "", false
}

// ReceiverInfo returns static information about the GPS receiver.
func (c *Configurator) ReceiverInfo() *gpsprot.ReceiverInfo {
	ver := c.ver
	var fwParts []string
	if ver.FW != nil {
		fwParts = append(fwParts, ver.FW.String())
	}
	if ver.Prot != nil {
		fwParts = append(fwParts, "PROTVER", ver.Prot.String())
	}

	rcvrInfo := gpsprot.ReceiverInfo{
		Vendor:         "u-blox",
		Hardware:       ver.Mod,
		Firmware:       strings.Join(fwParts, " "),
		SupportedGNSS:  ver.GNSS,
		VendorSpecific: ver,
	}

	// MON-GNSS provides supported GNSS directly rather than requiring us to parse them out,
	// so I think more reliable if we have it.
	if c.monGNSS != nil {
		rcvrInfo.SupportedGNSS = c.monGNSS.supportedGNSS
	}

	return &rcvrInfo
}

// GetRequestCount returns the current number of requests and whether the slice is complete.
func (c *Configurator) GetRequestCount() (count int, complete bool) {
	return len(c.reqs), c.complete
}

// Request returns the ConfigRequest at the given index.
func (c *Configurator) Request(index int) gpsprot.ConfigRequest {
	if index >= len(c.reqs) {
		panic("index >= GetRequestCount()")
	}
	return c.reqs[index]
}

// GenerateRequests attempts to generate more requests, potentially increasing the slice size.
func (c *Configurator) GenerateRequests() error {
	// First, handle state transitions and sequential progression
	// Note: updateRequestStates is not needed here as pause completion
	// is handled by the client via SetDeadlinePassed
	c.advanceSequentialProgress()

	// Only generate new requests if all existing requests are complete
	// (nextIndex has caught up to the end of the request slice)
	if c.nextIndex < len(c.reqs) {
		return nil
	}

	// Then generate new requests lazily using the existing step-based approach
	initialRequestCount := len(c.reqs)

	for c.stepIndex < len(c.steps) {
		err := c.steps[c.stepIndex](c)
		c.stepIndex++
		if err != nil {
			return err
		}

		// If we generated new requests, return to allow processing them
		// before generating more (subsequent requests may depend on results)
		if len(c.reqs) > initialRequestCount {
			// After generating new requests, ensure sequential progress is updated
			c.advanceSequentialProgress()
			return nil
		}
	}

	c.complete = true
	return nil
}

// advanceSequentialProgress advances nextIndex and makes next requests ready
func (c *Configurator) advanceSequentialProgress() {
	for c.nextIndex < len(c.reqs) {
		cr := c.reqs[c.nextIndex]

		// If current request succeeded, advance to next
		if cr.state == stateSucceeded {
			c.nextIndex++
			// Make next request ready if it exists
			if c.nextIndex < len(c.reqs) {
				c.reqs[c.nextIndex].state = stateReadyToSend
			}
		} else if cr.state == stateFailed {
			// Handle failed request - trigger recovery logic
			c.handleFailedRequest(c.nextIndex)
			break // Stop sequential progress after failure
		} else if cr.state == stateNotReady {
			// This request needs to become ready
			cr.state = stateReadyToSend
			break
		} else {
			break // Current request not ready to advance
		}
	}
}

// handleFailedRequest handles recovery logic when a request fails
func (c *Configurator) handleFailedRequest(index int) {
	// Recovery logic formerly in c.stop() method
	// Mark remaining requests as skipped
	for i := index + 1; i < len(c.reqs); i++ {
		if c.reqs[i].state == stateNotReady {
			c.reqs[i].state = stateSkipped
		}
	}
	// Move past the failed request to avoid reprocessing
	c.nextIndex = index + 1
	// Stop all further step processing - recovery should be final
	c.stepIndex = len(c.steps)
	// Mark as complete since we've handled failure and will add any recovery requests below
	c.complete = true

	// Perform recovery if needed (from old stop() method)
	// only need to do recovery for legacy configuration
	if &c.steps[0] != &legacyConfigSteps[0] {
		return
	}
	// if we didn't cause NMEA output to be disabled, then we don't need to perform recovery
	if !c.raw.prtNMEAOutDisabled(c.origPrt) {
		return
	}
	// if we got far enough to enable a message, then do not need to switch back to NMEA
	if c.raw.anyMsgEnabled() {
		return
	}
	// we disabled NMEA output, but didn't get far enough to enable the time GNSS message
	// we had better reenable NMEA output, or the GPS will be silent
	msg := c.origPrt
	c.reqs = append(c.reqs, &configRequest{
		ops:   msgSetRequest{msgRequest{msg}, &c.raw},
		cfg:   c,
		state: stateReadyToSend,
	})
}

// addRequest adds a new request to the Configurator interface
func (c *Configurator) addRequest(ops requestOps) error {
	c.reqs = append(c.reqs, &configRequest{
		ops:   ops,
		cfg:   c,
		state: stateNotReady,
	})
	return nil
}

// processAckNak handles both positive and negative acknowledgments
func (c *Configurator) processAckNak(msgID ubxbin.MsgID, ok bool, t time.Time, eventSeq uint64) {
	// Find the request that matches this ACK/NACK
	for i := 0; i < len(c.reqs); i++ {
		cr := c.reqs[i]
		// Check if this request is awaiting an ACK
		if cr.state == stateAwaitingAck || cr.state == stateAwaitingAckAndResponse {
			packet := cr.ops.Packet()
			if ubxbin.PacketMsgId(packet) == msgID && eventSeq > cr.sentSeq {
				if ok {
					// ACK received
					if cr.state == stateAwaitingAckAndResponse {
						// Still waiting for response
						cr.state = stateAwaitingResponse
						cr.checkComplete(t)
					} else {
						// Only waiting for ACK
						cr.checkComplete(t)
					}
				} else {
					// NACK received
					cr.state = stateFailed
					cr.err = fmt.Errorf("GPS receiver sent NACK for request %s", cr.ops.ID())
				}
				break
			}
		}
	}
}

// checkPollResponses checks if any awaiting poll requests are now satisfied.
// It intentionally skips stateAwaitingAck so that a response event cannot
// complete a request that is only waiting for an ACK.
func (c *Configurator) checkPollResponses(t time.Time) {
	for i := 0; i < len(c.reqs); i++ {
		cr := c.reqs[i]
		if cr.state == stateAwaitingAck {
			continue
		}
		cr.checkComplete(t)
	}
}

func (c *Configurator) processMsg(msg ubxbin.Msg, t time.Time) (bool, error) {
	switch mt := msg.(type) {
	case *ubxbin.AckAck:
		c.processAckNak(mt.MsgID, true, t, c.nextEventSeq())
		return true, nil
	case *ubxbin.AckNak:
		c.processAckNak(mt.MsgID, false, t, c.nextEventSeq())
		return true, nil
	case *ubxbin.MonComms:
		c.noteMsg(mt.ID(), t)
		if pid, ok := monCommsPort(mt); ok {
			c.portID = &pid
		}
		c.checkPollResponses(t)
		return true, nil
	case *ubxbin.MonGnss:
		c.noteMsg(mt.ID(), t)
		c.monGNSS = c.newMonGNSS(mt)
		c.checkPollResponses(t)
		return true, nil
	case *ubxbin.MonGnss1:
		c.noteMsg(mt.ID(), t)
		c.planSignals = monGnss1Signals(mt)
		c.checkPollResponses(t)
		return true, nil
	}
	mid := msg.ID()
	if mid.CfgClass() {
		c.noteMsg(mid, t)
		_, err := c.raw.AddMsg(msg)
		c.checkPollResponses(t)
		return err == nil, err
	}
	return false, nil
}

func (c *Configurator) newMonGNSS(mt *ubxbin.MonGnss) *monGNSS {
	mg := monGNSS{
		maxSimultaneousMajorGNSS: int(mt.Simultaneous),
		supportedGNSS:            monGNSSSet(mt.Supported),
		enabledGNSS:              monGNSSSet(mt.Enabled),
		gnssChangeCount:          c.raw.gnssChangeCount,
	}
	return &mg
}

// monCommsPort extracts the port ID from a MON-COMMS message.
// It first tries the output port from TxErrors, then falls back to
// a single-port device where only one port is reported.
func monCommsPort(mc *ubxbin.MonComms) (ubxbin.PortID, bool) {
	// The output port feature is documented for protocol version 40,
	// but returns not available on F10N and F10T.
	// It seems to work properly on protocol version 50 (X20 series).
	if pid, ok := mc.TxErrors.OutputPort(); ok {
		return pid, true
	}
	// This is a rather hacky fallback that works on F10N and F10T:
	// they only report one port (the active port), and we can use the port ID of that.
	// The point of this is to be able to test the MON-COMMS config path using F10N and F10T (which I have access to),
	// by changing pollMonComms to poll MON-COMMS for protocol version 40 and above.
	if len(mc.Ports) == 1 {
		return mc.Ports[0].PortID.PortID()
	}
	return 0, false
}

func (c *Configurator) saveMinimal() error {
	// This is just for legacy configuration.
	// For new configuration, we use CFG-VALSET to save to the right layer.
	var saveMask ubxbin.CfgCfgSectionMask
	if c.target.Opts.Save != gpsprot.SaveMinimal {
		return nil
	}
	// Port has bits for wther NMEA/RTCM output is enabled at all on the port.
	if c.target.Props.SetsAny(gpsprot.PropIDBaudRate) || c.target.Opts.NMEAMsg.IsSet() || c.target.Opts.RTCMMsg.IsSet() {
		saveMask |= ubxbin.CfgCfgIOPort
	}
	if c.target.Opts.SetsMsgs() {
		saveMask |= ubxbin.CfgCfgMsgConf
	}
	if c.target.Props.SetsAny(gpsprot.PropIDSignalsEnabled) || c.target.Props.SetsAny(cfgOldProps.tp5...) {
		saveMask |= ubxbin.CfgCfgRXMConf
	}
	// If any messages are enabled, then the rate is set, which is part of the Nav configuration section.
	// c.survey case handles where SetStatic resulted in a survey
	if c.target.Props.SetsAny(cfgOldProps.nav5...) || c.target.Props.SetsAny(cfgOldProps.tmode...) || c.survey || c.target.Opts.EnablesMsgs() {
		saveMask |= ubxbin.CfgCfgNavConf
	}
	if saveMask == 0 {
		return nil
	}
	return c.addRequest(msgRequest{c.newCfgCfgRequest(0, saveMask, 0, ubxbin.CfgCfgDevFlash|ubxbin.CfgCfgDevBBR)})
}

func (c *Configurator) setCfg() error {
	var saveMask, clearMask ubxbin.CfgCfgSectionMask

	if c.target.Opts.Save == gpsprot.SaveAll {
		saveMask = ubxbin.CfgCfgSectionMaskAll
	}
	if c.target.Opts.Reset == gpsprot.ResetFactory {
		clearMask = ubxbin.CfgCfgSectionMaskAll
	}
	if clearMask == 0 && saveMask == 0 {
		return nil
	}
	return c.addRequest(msgRequest{c.newCfgCfgRequest(clearMask, saveMask, 0, ubxbin.CfgCfgDevFlash|ubxbin.CfgCfgDevBBR)})
}

func (c *Configurator) reloadCfg() error {
	if c.target.Opts.Reset != gpsprot.ResetReload {
		return nil
	}
	return c.addRequest(msgRequest{c.newCfgCfgRequest(0, 0, ubxbin.CfgCfgSectionMaskAll, 0)})
}

func (*Configurator) newCfgCfgRequest(clearMask, saveMask, loadMask ubxbin.CfgCfgSectionMask, deviceMask ubxbin.CfgCfgDeviceMask) *ubxbin.CfgCfg {
	return &ubxbin.CfgCfg{
		CfgCfgFixed: ubxbin.CfgCfgFixed{
			ClearMask: clearMask,
			SaveMask:  saveMask,
			LoadMask:  loadMask,
		},
		DeviceMask: []ubxbin.CfgCfgDeviceMask{deviceMask},
	}
}

func (c *Configurator) reset() error {
	if c.target.Opts.Reset <= gpsprot.ResetReload {
		return nil
	}
	return c.addRequest(msgRequest{&ubxbin.CfgRst{
		NavBbrMask: ubxbin.CfgRstNavBbrColdStart,
		ResetMode:  ubxbin.CfgRstResetModeHardwareResetImmediately,
	}})
}

func (c *Configurator) valGet() error {
	port, portOK := c.valPort()
	_, missing, err := c.raw.valsPtr().Transaction(c.target, c.ver, port, portOK, c.monEnabledGNSS())
	if err != nil {
		return err
	}
	if len(missing) == 0 {
		return nil
	}
	return c.addMsgPollRequest(newCfgValgetRequest(missing, c.valGetLayer()))
}

// valPollMonGNSS polls UBX-MON-GNSS for non-legacy configuration.
func (c *Configurator) valPollMonGNSS() error {
	// MON-GNSS is use to determine enabled GNSS when configuring RTCM messages.
	// XXX in the future use for maxSimultaneousMajorGNSS (needed for F10T at least)
	// XXX in the future use also when inferring time GNSS
	// Also X20P and newer (UBX protocol version 50 and newer) support a new MON-GNSS message that properly provides the supported signals;
	// we use that to determine supported signals when setting the enabled signals.
	if c.target.Opts.RTCMMsg.IsSet() || (c.ver.protVerAtLeast(50, 0) && c.target.Props.SetsAny(gpsprot.PropIDSignalsEnabled)) {
		return c.addPollRequest(ubxbin.MonGnssID)
	}
	return nil
}

func (c *Configurator) valGetSignals() error {
	if !c.target.UsesAny(gpsprot.PropIDSignalsEnabled) {
		return nil
	}
	keys := []ucv.Key{
		ucv.KSignalGpsEna.Key().GroupWildcard(),
		ucv.KGpsL5HealthOverride.Key().GroupWildcard(),
	}
	return c.addMsgPollRequest(newCfgValgetRequest(keys, c.valGetLayer()))
}

func (c *Configurator) valSetSignals() error {
	targetEnabled, ok := c.target.Props.GetSignalsEnabled()
	if !ok {
		return nil
	}
	supported := andIfNonZero(c.planSignals, c.raw.valsPtr().signalsSupported(c.ver))
	if supported == 0 {
		return errors.New("could not determine supported GNSS signals")
	}
	enabled, items := c.raw.valsPtr().EnableSignals(targetEnabled, supported)
	// Ensure we have one non-augmentation signal from a major GNSS
	enabled &= gpsprot.SigSetMajor
	enabled &^= gpsprot.SigSetAugment
	if enabled == 0 {
		return fmt.Errorf("no suitable supported GNSS signal was enabled: %v", enabled)
	}
	val, err := newCfgValsetRequest(items, c.valSetLayer())
	if err != nil {
		return err
	}
	return c.addMsgSetPauseRequest(val, pauseAfterGNSSReset)
}

func andIfNonZero(ss1, ss2 gpsprot.SignalSet) gpsprot.SignalSet {
	if ss1 == 0 {
		return ss2
	}
	if ss2 == 0 {
		return ss1
	}
	return ss1 & ss2
}

func (c *Configurator) valGetNMA() error {
	if !c.target.UsesAny(gpsprot.PropIDNavMsgAuth) {
		return nil
	}
	keys := []ucv.Key{
		ucv.KGalUseOsnma.Key().GroupWildcard(),
	}
	return c.addMsgPollRequest(newCfgValgetRequest(keys, c.valGetLayer()))
}

func (c *Configurator) valSetNMA() error {
	items := c.raw.valsPtr().NavMsgAuth(&c.target.Props)
	if len(items) == 0 {
		return nil
	}
	val, err := newCfgValsetRequest(items, c.valSetLayer())
	if err != nil {
		return nil
	}
	return c.addMsgSetRequest(val)
}

func (c *Configurator) valSet() error {
	port, portOK := c.valPort()
	items, missing, err := c.raw.valsPtr().Transaction(c.target, c.ver, port, portOK, c.monEnabledGNSS())
	if err != nil {
		return err
	}
	if len(missing) != 0 {
		return fmt.Errorf("missing config items: %v", missing)
	}
	if len(items) == 0 {
		return nil
	}
	val, err := newCfgValsetRequest(items, c.valSetLayer())
	if err != nil {
		return err
	}
	return c.addMsgSetRequest(val)
}

func (c *Configurator) valGetLayer() ubxbin.CfgValgetLayer {
	return ubxbin.CfgValgetLayerRAM
}

func (c *Configurator) valSetLayer() ubxbin.CfgValsetLayer {
	if c.target.Opts.Save == gpsprot.SaveMinimal {
		// SaveMinimal is implemented by writing to non-volatile layers.
		// SaveAll is implemented using UBX-CFG-CFG to save everything
		return ubxbin.CfgValsetLayerFlash | ubxbin.CfgValsetLayerBBR | ubxbin.CfgValsetLayerRAM
	}
	return ubxbin.CfgValsetLayerRAM
}

func (c *Configurator) monEnabledGNSS() gpsprot.GNSSSet {
	if c.monGNSS != nil && c.monGNSS.gnssChangeCount == c.raw.gnssChangeCount {
		return c.monGNSS.enabledGNSS
	}
	return 0
}

func (c *Configurator) valBaudRate() error {
	if !c.target.Props.SetsAny(gpsprot.PropIDBaudRate) {
		return nil
	}
	port, ok := c.valPort()
	if !ok {
		return errors.New("baud-rate change requested but receiver port unknown")
	}
	items := c.raw.valsPtr().BaudRate(c.target, port)
	if len(items) == 0 {
		return nil
	}
	val, err := newCfgValsetRequest(items, c.valSetLayer())
	if err != nil {
		return nil
	}
	return c.addMsgSetSpeedRequest(val, int(items[0].Value))
}

// valPort returns the active receiver port and true when known, or
// (0, false) when it has not been discovered. The zero value must not
// be treated as a guess: callers that need a real port for a write
// must surface an error on !ok rather than silently substituting one.
func (c *Configurator) valPort() (ucv.Port, bool) {
	if c.portID != nil {
		return ucv.Port(*c.portID), true
	}
	if c.raw.prt != nil {
		return ucv.Port(c.raw.prt.PortID), true
	}
	return 0, false
}

func newCfgValgetRequest(keys []ucv.Key, layer ubxbin.CfgValgetLayer) *ubxbin.CfgValget {
	return &ubxbin.CfgValget{
		CfgValgetFixed: ubxbin.CfgValgetFixed{
			Layer:   layer,
			Version: ubxbin.CfgValgetVersionRequest,
		},
		CfgData: ucv.MarshalKeys(keys),
	}
}

func newCfgValsetRequest(items []ucv.Item, layers ubxbin.CfgValsetLayer) (*ubxbin.CfgValset, error) {
	cfgData, err := ucv.MarshalItems(items)
	if err != nil {
		return nil, err
	}
	return &ubxbin.CfgValset{
		CfgValsetFixed: ubxbin.CfgValsetFixed{
			Layers:  layers,
			Version: ubxbin.CfgValsetVersionNoTransaction,
		},
		CfgData: cfgData,
	}, nil
}

func (c *Configurator) pollMonComms() error {
	// UBX-MON-COMMS works on protocol version 40 (10th gen)
	// However, CFG-PRT works on anything less than protocol version 50 (X20 series)
	// Also, the current port feature of UBX-MON-COMMS that is documented for version 40,
	// only actually seems to work fully in version 50.
	// You can change this temporarile to 40 to test on F10N/F10T devices.
	// But for normal operation, I think it is safer to use UBX-MON-COMMS on protocol version 50 and above.
	if !c.ver.protVerAtLeast(50, 0) {
		return nil
	}
	if !c.needsPort() {
		return nil
	}
	return c.addPollRequest(ubxbin.MonCommsID)
}

func (c *Configurator) pollPrt() error {
	if c.portID != nil {
		return nil
	}
	if !c.needsPort() {
		return nil
	}
	return c.addPollRequest(ubxbin.CfgPrtID)
}

// needsPort reports whether any downstream step requires the active
// receiver port. PropIDPort is read-only so target.Get is checked
// directly rather than via UsesAny.
func (c *Configurator) needsPort() bool {
	if c.target.Get&gpsprot.PropIDPort != 0 {
		return true
	}
	return c.target.UsesAny(gpsprot.PropIDBaudRate) || c.target.Opts.SetsMsgs()
}

func (c *Configurator) pollGNSS() error {
	// UBX-CFG-GNSS needs at least protocol version 14.00
	if !c.target.UsesAny(cfgOldProps.gnss...) || !c.ver.protVerAtLeast(14, 0) {
		return nil
	}
	return c.addPollRequest(ubxbin.CfgGNSSID)
}

// This is just for legacy configuration.
// It is called after polling GNSS (if any)
func (c *Configurator) pollMonGNSS() error {
	// UBX-MON-GNSS needs at least protocol version 15.00
	if !c.ver.protVerAtLeast(15, 0) {
		return nil
	}
	if !c.needsMonGNSS() {
		return nil
	}
	return c.addPollRequest(ubxbin.MonGnssID)
}

func (c *Configurator) needsMonGNSS() bool {
	// Need maxSimultaneousMajorGNSS in this case.
	if _, ok := c.target.Props.GetSignalsEnabled(); ok {
		return true
	}
	// Need enabledGNSS for RTCM messages.
	// But only need it if we can't get it from raw.gnss.
	if c.target.Opts.RTCMMsg.IsSet() && c.raw.gnss == nil {
		return true
	}
	return false
}

func (c *Configurator) pollRate() error {
	// XXX also handle survey message
	if _, ok := c.target.Props.GetTimeGNSS(); !ok && !c.target.Opts.EnablesMsgs() {
		return nil
	}
	return c.addPollRequest(ubxbin.CfgRateID)
}

func (c *Configurator) pollNav5() error {
	if !c.target.UsesAny(cfgOldProps.nav5...) &&
		// this handles when we need to set dynmodel
		!(dynModelStatic(c.target) != nil && c.ver.tmodeLevel() == 0) {
		return nil
	}
	return c.addPollRequest(ubxbin.CfgNav5ID)
}

func (c *Configurator) pollTmode() error {
	if !c.target.UsesAny(cfgOldProps.tmode...) && !c.target.Opts.SetStatic {
		return nil
	}
	switch c.ver.tmodeLevel() {
	case 1:
		return c.addPollRequest(ubxbin.CfgTmodeID)
	case 2:
		return c.addPollRequest(ubxbin.CfgTmode2ID)
	case 3:
		return c.addPollRequest(ubxbin.CfgTmode3ID)
	}
	return nil
}

func (c *Configurator) pollTp5() error {
	if !c.target.UsesAny(cfgOldProps.tp5...) {
		return nil
	}
	return c.addPollTp5Request(c.ver.tpIndex())
}

func (c *Configurator) setMsg1() error {
	if c.raw.prt != nil {
		// save the original port configuration
		// so we can restore during recovery
		c.origPrt = new(ubxbin.CfgPrt)
		*c.origPrt = *c.raw.prt
	}
	mc := newMsgChanges()
	mc.options1(&c.target.Opts, c.ver)
	c.setMsgChanges(mc)
	return nil
}

func (c *Configurator) setMsg2() error {
	mc := newMsgChanges()
	enabledGNSS := c.monEnabledGNSS()
	if enabledGNSS == 0 {
		enabledGNSS = gnssEnabledSet(c.raw.gnss)
	}
	err := mc.options2(&c.target.Opts, c.ver, enabledGNSS, c.survey)
	if err != nil {
		return err
	}
	// this will end up doing about CFG-PRT in the event that RTCM protocol is enable/disabled
	c.setMsgChanges(mc)
	return nil
}

func (c *Configurator) setMsgChanges(mc *msgChanges) {
	prt := c.raw.changePrtProto(mc)
	if prt != nil {
		c.addMsgSetRequest(prt)
	}
	for msgID, rate := range mc.rates() {
		c.addMsgRateRequest(msgID, rate)
	}
}

func (c *Configurator) setBaudRate() error {
	prt := c.raw.changePrtBaudRate(c.target)
	if prt == nil {
		return nil
	}
	baudRate, _ := c.target.Props.GetBaudRate()
	return c.addMsgSetSpeedRequest(prt, int(baudRate))
}

func (c *Configurator) setNav5() error {
	nav5 := c.raw.changeNav5(c.target, c.ver)
	if nav5 == nil {
		return nil
	}
	// XXX this isn't quite right, because of the mask
	// msgSetRequest should pass a flag to AddMsg saying whether it came from a get or a set
	// Then on a set AddMsg for nav5 should only update things according to the mask
	return c.addMsgSetRequest(nav5)
}

func (c *Configurator) setRate() error {
	rate := c.raw.changeRate(&c.target.Props)
	if rate == nil {
		return nil
	}
	return c.addMsgSetRequest(rate)
}

func (c *Configurator) setTmode() error {
	msg1, msg2, err := c.raw.changeTmode(c.target)
	if err != nil {
		return err
	}
	if msg1 != nil {
		c.addMsgSetRequest(msg1)
	}
	if msg2 != nil {
		c.survey = true // this is needed for enabling survey messages
		c.addMsgSetRequest(msg2)
	}
	return nil
}

func (c *Configurator) setTp5() error {
	tp5 := c.raw.changeTp5(&c.target.Props)
	if tp5 == nil {
		return nil
	}
	return c.addMsgSetRequest(tp5)
}

func (c *Configurator) setGNSS() error {
	gnss, err := c.raw.changeGNSS(&c.target.Props, c.ver, c.monGNSS)
	if gnss == nil || err != nil {
		return err
	}
	return c.addMsgSetPauseRequest(gnss, pauseAfterGNSSReset)
}

func (c *Configurator) timeAssist() error {
	mga, err := mgaTime(&c.target.Opts.TimeAssist, time.Now())
	if err != nil {
		return err
	}
	if mga == nil {
		return nil
	}
	return c.addRequest(msgRequest{mga})
}

func (c *Configurator) osnmaAssist() error {
	mga := mgaOSNMAMerkle(c.target.Opts.OSNMA.MerkleTreeRoot, false)
	if mga == nil {
		return nil
	}
	return c.addRequest(msgRequest{mga})
}

func (raw *RawConfig) Config(ver *Version, portID *ubxbin.PortID) *gpsprot.ConfigProps {
	if raw == nil {
		return nil
	}
	cm := &gpsprot.ConfigProps{}
	if !raw.CfgVals.isNil() {
		// Active port for val-based: MON-COMMS first, CFG-PRT second.
		port := portID
		if port == nil && raw.prt != nil {
			port = &raw.prt.PortID
		}
		raw.CfgVals.Cook(ver, cm, port)
	} else {
		raw.cookTmode(cm)
		raw.cookTp5(cm)
		raw.cookGNSS(cm)
		// must call cookNav5 after cookTp5, because we want to prefer primary GNSS from TP5
		raw.cookNav5(cm, ver)
		raw.cookPrt(cm)
	}
	return cm
}

func (raw *RawConfig) AddMsg(m ubxbin.Msg) (bool, error) {
	if raw == nil {
		return false, nil
	}
	switch mt := m.(type) {
	case *ubxbin.CfgTmode:
		raw.tmode = mt
	case *ubxbin.CfgTmode2:
		raw.tmode2 = mt
	case *ubxbin.CfgTmode3:
		raw.tmode3 = mt
	case *ubxbin.CfgTp5:
		raw.tp5 = mt
	case *ubxbin.CfgGNSS:
		raw.gnss = mt
		raw.gnssChangeCount++
	case *ubxbin.CfgRate:
		raw.rate = mt
	case *ubxbin.CfgNav5:
		raw.nav5 = mt
	case *ubxbin.CfgPrt:
		raw.prt = mt
	case *ubxbin.CfgMsg:
		raw.addMsgRate(mt.MsgID, mt.Rate)
	case *ubxbin.CfgValget:
		// this is a response to a poll
		if mt.Layer == ubxbin.CfgValgetLayerRAM {
			_, err := raw.valsPtr().AddData(mt.CfgData)
			if err != nil {
				return false, err
			}
		}
	case *ubxbin.CfgValset:
		// this is an acknowledgement of a set
		if mt.Layers&ubxbin.CfgValsetLayerRAM != 0 {
			groups, err := raw.valsPtr().AddData(mt.CfgData)
			if err != nil {
				return false, err
			}
			if _, found := groups[ucv.KSignalGpsEna.Key().Group()]; found {
				raw.gnssChangeCount++
			}
		}
	default:
		return false, nil
	}
	return true, nil
}

type msgRequest struct {
	msg ubxbin.Msg
}

var _ requestOps = (*msgRequest)(nil)

func (r msgRequest) Packet() []byte {
	pkt, err := ubxbin.Serialize(r.msg)
	if err != nil {
		panic(err)
	}
	return pkt
}

func (r msgRequest) ChangeSpeed() int { return 0 }

func (r msgRequest) Pause() time.Duration { return 0 }

func (r msgRequest) ID() string { return r.msg.ID().String() }

func (r msgRequest) Ackable() bool { return r.msg.ID().Ackable() }

func (r msgRequest) AwaitingResponse(uint64) bool { return false }

func (r msgRequest) Done() {}

func (c *Configurator) addMsgSetRequest(msg ubxbin.Msg) error {
	return c.addRequest(msgSetRequest{msgRequest{msg}, &c.raw})
}

func (c *Configurator) addMsgSetSpeedRequest(msg ubxbin.Msg, speed int) error {
	return c.addRequest(msgSetSpeedRequest{msgSetRequest{msgRequest{msg}, &c.raw}, speed})
}

func (c *Configurator) addMsgSetPauseRequest(msg ubxbin.Msg, pause time.Duration) error {
	return c.addRequest(msgSetPauseRequest{msgSetRequest{msgRequest{msg}, &c.raw}, pause})
}

func (c *Configurator) addMsgPollRequest(msg ubxbin.Msg) error {
	return c.addRequest(msgPollRequest{msgRequest: msgRequest{msg}, msgSeq: c.msgSeq})
}

func (c *Configurator) addPollRequest(mid ubxbin.MsgID) error {
	return c.addRequest(pollRequest{c.msgSeq, mid})
}

func (c *Configurator) addPollTp5Request(tpIdx int) error {
	return c.addRequest(pollTp5Request{
		pollRequest: pollRequest{c.msgSeq, ubxbin.CfgTp5ID},
		tpIdx:       tpIdx,
	})
}

type msgPollRequest struct {
	msgRequest
	msgSeq map[ubxbin.MsgID]uint64
}

func (r msgPollRequest) AwaitingResponse(sentSeq uint64) bool {
	return r.msgSeq[r.msg.ID()] <= sentSeq
}

var _ requestOps = (*msgPollRequest)(nil)

type msgSetRequest struct {
	msgRequest
	raw *RawConfig
}

var _ requestOps = (*msgSetRequest)(nil)

func (r msgSetRequest) Done() {
	_, err := r.raw.AddMsg(r.msg)
	if err != nil {
		panic(fmt.Sprintf("cannot parse acknowledge message %s: %v", r.msg.ID(), err))
	}
}

type msgSetSpeedRequest struct {
	msgSetRequest
	speed int
}

func (r msgSetSpeedRequest) ChangeSpeed() int {
	return r.speed
}

var _ requestOps = (*msgSetSpeedRequest)(nil)

const pauseAfterGNSSReset = time.Second / 2

// speedChangeDelay is how long after sending a speed change command until we can
// accept a valid packet as confirmation that the speed change succeeded.
// This accounts for the time it takes the GPS to process and apply the speed change.
const speedChangeDelay = 400 * time.Millisecond

type msgSetPauseRequest struct {
	msgSetRequest
	pause time.Duration
}

func (r msgSetPauseRequest) Pause() time.Duration {
	return r.pause
}

var _ requestOps = (*msgSetPauseRequest)(nil)

type pollRequest struct {
	msgSeq map[ubxbin.MsgID]uint64
	msgID  ubxbin.MsgID
}

func (r pollRequest) Packet() []byte {
	return ubxbin.Poll(r.msgID)
}

func (r pollRequest) ChangeSpeed() int { return 0 }

func (r pollRequest) Pause() time.Duration { return 0 }

func (r pollRequest) ID() string { return r.msgID.String() }

func (r pollRequest) Done() {}

func (r pollRequest) Ackable() bool {
	return r.msgID.Ackable()
}

func (r pollRequest) AwaitingResponse(sentSeq uint64) bool {
	return r.msgSeq[r.msgID] <= sentSeq
}

type pollTp5Request struct {
	pollRequest
	tpIdx int
}

func (r pollTp5Request) Packet() []byte {
	return ubxbin.PollCfgTp5(r.tpIdx)
}

func (c *Configurator) addMsgRateRequest(msgID ubxbin.MsgID, rate MsgRate) error {
	return c.addRequest(msgRateRequest{&c.raw, msgID, uint8(rate)})
}

type msgRateRequest struct {
	raw   *RawConfig
	msgID ubxbin.MsgID
	rate  byte
}

func (r msgRateRequest) Packet() []byte {
	return ubxbin.SetCfgMsg(r.msgID, r.rate)
}

func (r msgRateRequest) ChangeSpeed() int { return 0 }

func (r msgRateRequest) Pause() time.Duration { return 0 }

func (r msgRateRequest) Done() {
	r.raw.SetMsgRate(r.msgID, r.rate)
}

func (r msgRateRequest) Ackable() bool { return true }

func (r msgRateRequest) AwaitingResponse(uint64) bool { return false }

func (r msgRateRequest) ID() string { return ubxbin.CfgMsgID.String() }

// lengthHP converts a high-precision coordinate (cm main + 0.1mm HP)
// used by NAV-HPPOSECEF for ECEF coordinates.
func lengthHP(l int32, h int8) gpsprot.Length {
	return gpsprot.Length(l)*gpsprot.Centimeter + gpsprot.Length(h)*(gpsprot.Millimeter/10)
}

// lengthHPmm converts a high-precision height (mm main + 0.1mm HP)
// used by NAV-HPPOSLLH for height and hMSL.
func lengthHPmm(l int32, h int8) gpsprot.Length {
	return gpsprot.Length(l)*gpsprot.Millimeter + gpsprot.Length(h)*(gpsprot.Millimeter/10)
}

func angleHP(deg int32, hp int8) gpsprot.Angle {
	return gpsprot.Angle(deg)*(gpsprot.Nanodegrees*100) + gpsprot.Angle(hp)*gpsprot.Nanodegrees
}

func angleToInt8Degrees(a gpsprot.Angle) (int8, bool) {
	// Ceil so that e.g. 5.4 degrees becomes 6, not 5 -- rounding down would admit
	// satellites the user explicitly wanted excluded.
	v := int64(math.Ceil(a.Degrees()))
	if v < -90 || v > 90 {
		return 0, false
	}
	return int8(v), true
}
