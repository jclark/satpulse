package quectel

import (
	"fmt"
	"strings"
	"time"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/internal/nmea"
	"github.com/jclark/satpulse/gps/lib/nmeamsg"
	"github.com/jclark/satpulse/gps/lib/qtmmsg"
)

const Vendor = "Quectel"

// configSupport is everything except raw observation output (the
// protocol has none; RTCM MSM is the only observation carrier) and a
// fixed-position accuracy input (CFGSVIN's accuracy limit applies to
// Survey-in only). Signal, positioning-mode, and RTCM-MSM-type
// changes are stored-only on this receiver (they take effect at the
// next NVM reload, never live), so all three carry the
// only-with-reset qualifier.
const configSupport gpsprot.ConfigSupportFlags = gpsprot.ConfigSupportFull&^
	(gpsprot.ConfigSupportRaw|gpsprot.ConfigSupportFixedPosAcc) |
	gpsprot.ConfigSupportSignalOnlyWithReset | gpsprot.ConfigSupportModeOnlyWithReset |
	gpsprot.ConfigSupportRTCMMSMOnlyWithReset

const (
	// responseTimeout is the per-attempt response window. Responses
	// normally arrive within a few ms but can stall ~100 ms behind the
	// periodic output burst; patience beyond this comes from the
	// director's retry budget.
	responseTimeout = 2 * time.Second
	// speedChangeRepeatDelay is how long after sending a speed change to
	// wait for its OK before repeating the write. The port switches ~2 ms
	// after the request, so the repeat is received intact at the new rate.
	speedChangeRepeatDelay = 100 * time.Millisecond
	// speedChangeDelay is how long after sending a speed change until an
	// incidental valid packet confirms it: the host switches rates right
	// after the send, so a packet read this long afterwards can only have
	// arrived at the new rate.
	speedChangeDelay = 400 * time.Millisecond
)

// ConfigProtocol implements gpsprot.ConfigProtocol for Quectel PQTM
// text configuration (LG290P and family).
type ConfigProtocol struct {
	verno  *qtmmsg.Verno
	sawErr bool // a PQTMVERNO,ERROR reply also proves a PQTM speaker
	cfg    *Configurator
}

// Compile-time interface compliance checks
var _ gpsprot.ConfigProtocol = (*ConfigProtocol)(nil)
var _ gpsprot.Configurator = (*Configurator)(nil)
var _ gpsprot.ConfigRequest = (*request)(nil)

// NewConfigProtocol creates a ConfigProtocol for Quectel PQTM receivers.
func NewConfigProtocol() *ConfigProtocol {
	return &ConfigProtocol{}
}

// nmeaPacket frames an NMEA payload (between $ and *) into a
// transmittable packet.
func nmeaPacket(payload string) []byte {
	return fmt.Appendf(nil, "$%s*%02X\r\n", payload, nmeamsg.Checksum([]byte(payload)))
}

// ProbePackets returns the PQTMVERNO query. Every PQTM-speaking
// firmware answers it: with version data, or with ERROR for a
// hypothetical firmware lacking it - either reply proves the protocol.
func (cp *ConfigProtocol) ProbePackets() ([][]byte, time.Duration) {
	return [][]byte{nmeaPacket("PQTMVERNO")}, 0
}

// ProbeOK reports whether a PQTM response to the probe has been seen.
func (cp *ConfigProtocol) ProbeOK() bool {
	return cp.verno != nil || cp.sawErr
}

// NativeMsg processes NMEA sentences, consuming PQTMVERNO probe
// replies and routing other PQTM responses to the active
// configurator. The configurator never requests PQTMVERNO itself, so
// late probe replies collide with nothing.
func (cp *ConfigProtocol) NativeMsg(tag gpsprot.Tag, msgID string, msg any, tRead time.Time) error {
	sen, ok := msg.(*nmea.Sentence)
	if !ok {
		return nil
	}
	rc := qtmmsg.ClassifyResponse(sen.Payload)
	if rc.Kind == qtmmsg.ResponseNotPQTM {
		return nil
	}
	if rc.Sentence == "PQTMVERNO" {
		if rc.Kind == qtmmsg.ResponseError {
			cp.sawErr = true
			return nil
		}
		v, err := qtmmsg.ParseVerno(sen.Payload)
		if err != nil {
			return err
		}
		if v != nil {
			cp.verno = v
		}
		return nil
	}
	if cp.cfg != nil {
		return cp.cfg.response(rc, sen.Payload)
	}
	return nil
}

// Configure creates the Configurator for the given target.
func (cp *ConfigProtocol) Configure(target *gpsprot.ConfigTarget) (gpsprot.Configurator, error) {
	var fw fwVersion
	if cp.verno != nil {
		fw = parseFwVersion(cp.verno.VerStr)
	}
	cp.cfg = &Configurator{
		target: target,
		verno:  cp.verno,
		fw:     fw,
	}
	return cp.cfg, nil
}

// Configurator implements gpsprot.Configurator for PQTM receivers.
// It holds the receiver's as-found configuration as typed per-command
// tuples in the receiver's own vocabulary: nil means never read back,
// which surfaces as property absence.
type Configurator struct {
	phase    configPhase
	fw       fwVersion
	verno    *qtmmsg.Verno
	uniqid   *qtmmsg.UniqID
	found    asFound
	reqs     []*request
	target   *gpsprot.ConfigTarget
	complete bool
	// Lazy-generation flags: only GenerateRequests may grow the
	// request slice, so response handlers set these instead of
	// appending. protQueried marks the CFGPROT query, which needs the
	// active port index from the CFGUART answer; needPPS asks for the
	// legacy PPS query after a refused PPS2 query; speedReq is the
	// pending speed change, watched for a due repeat (see
	// generateSpeedChange).
	protQueried bool
	needPPS     bool
	speedReq    *request
	// msgWantState caches the desired message-output state computed
	// by msgWant, keyed by message name.
	msgWantState map[string]bool
}

type configPhase int

const (
	phaseQuery   configPhase = iota // reading as-found state
	phaseSet                        // property sets
	phaseMsg                        // message-output rate sets
	phaseSpeed                      // baud-rate change, before the save so a save persists the new rate
	phaseNVM                        // PQTMSAVEPAR, after everything it must persist
	phaseReset                      // PQTMRESTOREPAR, which acknowledges
	phaseRestart                    // PQTMSRR/PQTMCOLD, which answer nothing; last,
	// so a restart never races an acknowledgement (stage-0 lesson:
	// pipelining SAVEPAR with SRR destroys the save's OK)
	phaseFinal
)

// asFound is the receiver's as-found configuration, populated by the
// query phase's get responses.
type asFound struct {
	uart     *qtmmsg.CfgUART
	pps2     *qtmmsg.CfgPPS2
	pps      *qtmmsg.CfgPPS
	fixRate  *qtmmsg.CfgFixRate
	eleThd   *qtmmsg.CfgEleThd
	cnst     *qtmmsg.CfgCnst
	signal   *qtmmsg.CfgSignal
	svin     *qtmmsg.CfgSvin
	rcvrMode *qtmmsg.CfgRcvrMode
	prot     *qtmmsg.CfgProt
	rtcm     *qtmmsg.CfgRtcm
	rsid     *qtmmsg.CfgRSID
}

// request is a single PQTM exchange. Behavior flags select how
// responses and errors resolve it; the state field holds the
// gpsprot.ConfigRequestState directly.
type request struct {
	state    gpsprot.ConfigRequestState
	phase    configPhase // not sendable before this phase (zero = query)
	payload  string      // NMEA payload between $ and *
	sentence string      // correlation key: the sentence name
	noReply  bool        // resets and the speed-change repeat: answer nothing, succeed when sent
	speed    int         // new baud rate to switch to after sending (0 if none)
	// repeatDue is set on a speed change whose OK did not arrive within
	// speedChangeRepeatDelay; GenerateRequests responds by sending the
	// repeat while this request keeps awaiting with the full timeout.
	repeatDue bool
	onOK     func()                     // called on a bare OK acknowledgement
	onOKData func(payload string) error // called with the full response payload
	onError  func(rc qtmmsg.ResponseClass) bool // true = absorbed (absence); false = fail
	tBase    time.Time
	err      error
}

func (c *Configurator) queryReq(payload, sentence string, store func(qtmmsg.CfgMsg)) *request {
	return &request{
		payload:  payload,
		sentence: sentence,
		onOKData: func(p string) error {
			m, err := qtmmsg.ParseCfgResponse(p)
			if err != nil {
				return err
			}
			if m != nil {
				store(m)
			}
			return nil
		},
		// An unsupported or refused query means the feature does not
		// exist on this firmware: the tuple stays nil and the
		// properties it carries are absent.
		onError: func(rc qtmmsg.ResponseClass) bool { return true },
	}
}

// GenerateRequests generates the query wave, promotes sendable
// requests (single-flight per sentence name; distinct names
// pipeline), and advances phases when all requests are final.
func (c *Configurator) GenerateRequests() error {
	if len(c.reqs) == 0 {
		c.generateQueries()
	}
	if c.needPPS {
		c.needPPS = false
		c.reqs = append(c.reqs, c.ppsQuery())
	}
	if c.found.uart != nil && !c.protQueried {
		c.protQueried = true
		c.reqs = append(c.reqs, c.queryReq(
			fmt.Sprintf("PQTMCFGPROT,R,1,%d", c.found.uart.Index), "PQTMCFGPROT",
			func(m qtmmsg.CfgMsg) { c.found.prot = m.(*qtmmsg.CfgProt) }))
	}
	if c.phase == phaseQuery && c.allFinal() {
		c.phase = phaseSet
		if err := c.generatePropSets(); err != nil {
			return err
		}
	}
	if c.phase == phaseSet && c.allFinal() {
		c.phase = phaseMsg
		c.generateMsgSets()
	}
	if c.phase == phaseMsg && c.allFinal() {
		c.phase = phaseSpeed
		c.generateSpeedChange()
	}
	if c.speedReq != nil && c.speedReq.repeatDue {
		if c.speedReq.state == gpsprot.ConfigRequestAwaitingResponse {
			// Re-send the same write: the receiver is at the new rate by
			// now, so re-applying it is a no-op whose OK routes to the
			// original request (oldest live PQTMCFGUART). The repeat
			// itself expects no response.
			c.reqs = append(c.reqs, &request{phase: phaseSpeed,
				payload: c.speedReq.payload, sentence: c.speedReq.sentence, noReply: true})
		}
		c.speedReq = nil
	}
	if c.phase == phaseSpeed && c.allFinal() {
		c.phase = phaseNVM
		if c.target.Opts.Save != gpsprot.SaveNone {
			c.reqs = append(c.reqs, &request{phase: phaseNVM,
				payload: "PQTMSAVEPAR", sentence: "PQTMSAVEPAR"})
		}
	}
	if c.phase == phaseNVM && c.allFinal() {
		c.phase = phaseReset
		if c.target.Opts.Reset == gpsprot.ResetFactory {
			c.reqs = append(c.reqs, &request{phase: phaseReset,
				payload: "PQTMRESTOREPAR", sentence: "PQTMRESTOREPAR"})
		}
	}
	if c.phase == phaseReset && c.allFinal() {
		c.phase = phaseRestart
		switch c.target.Opts.Reset {
		case gpsprot.ResetReload, gpsprot.ResetFactory:
			c.reqs = append(c.reqs, &request{phase: phaseRestart,
				payload: "PQTMSRR", sentence: "PQTMSRR", noReply: true})
		case gpsprot.ResetCold:
			c.reqs = append(c.reqs, &request{phase: phaseRestart,
				payload: "PQTMCOLD", sentence: "PQTMCOLD", noReply: true})
		}
	}
	if c.phase == phaseRestart && c.allFinal() {
		c.phase = phaseFinal
		c.complete = true
	}
	c.promote()
	return nil
}

func (c *Configurator) generateQueries() {
	c.reqs = append(c.reqs, c.queryReq("PQTMCFGUART,R", "PQTMCFGUART",
		func(m qtmmsg.CfgMsg) { c.found.uart = m.(*qtmmsg.CfgUART) }))
	if c.fw.has(fwPPS2) {
		pps2 := c.queryReq("PQTMCFGPPS2,R,1", "PQTMCFGPPS2",
			func(m qtmmsg.CfgMsg) { c.found.pps2 = m.(*qtmmsg.CfgPPS2) })
		pps2.onError = func(rc qtmmsg.ResponseClass) bool {
			c.needPPS = true
			return true
		}
		c.reqs = append(c.reqs, pps2)
	} else {
		c.reqs = append(c.reqs, c.ppsQuery())
	}
	c.reqs = append(c.reqs, c.queryReq("PQTMCFGFIXRATE,R", "PQTMCFGFIXRATE",
		func(m qtmmsg.CfgMsg) { c.found.fixRate = m.(*qtmmsg.CfgFixRate) }))
	if c.fw.has(fwEleThd) {
		c.reqs = append(c.reqs, c.queryReq("PQTMCFGELETHD,R", "PQTMCFGELETHD",
			func(m qtmmsg.CfgMsg) { c.found.eleThd = m.(*qtmmsg.CfgEleThd) }))
	}
	c.reqs = append(c.reqs, c.queryReq("PQTMCFGCNST,R", "PQTMCFGCNST",
		func(m qtmmsg.CfgMsg) { c.found.cnst = m.(*qtmmsg.CfgCnst) }))
	c.reqs = append(c.reqs, c.queryReq("PQTMCFGSIGNAL,R", "PQTMCFGSIGNAL",
		func(m qtmmsg.CfgMsg) { c.found.signal = m.(*qtmmsg.CfgSignal) }))
	c.reqs = append(c.reqs, c.queryReq("PQTMCFGSVIN,R", "PQTMCFGSVIN",
		func(m qtmmsg.CfgMsg) { c.found.svin = m.(*qtmmsg.CfgSvin) }))
	c.reqs = append(c.reqs, c.queryReq("PQTMCFGRCVRMODE,R", "PQTMCFGRCVRMODE",
		func(m qtmmsg.CfgMsg) { c.found.rcvrMode = m.(*qtmmsg.CfgRcvrMode) }))
	c.reqs = append(c.reqs, c.queryReq("PQTMCFGRTCM,R", "PQTMCFGRTCM",
		func(m qtmmsg.CfgMsg) { c.found.rtcm = m.(*qtmmsg.CfgRtcm) }))
	c.reqs = append(c.reqs, c.queryReq("PQTMCFGRSID,R", "PQTMCFGRSID",
		func(m qtmmsg.CfgMsg) { c.found.rsid = m.(*qtmmsg.CfgRSID) }))
	uniqid := &request{
		payload:  "PQTMUNIQID",
		sentence: "PQTMUNIQID",
		onOKData: func(p string) error {
			u, err := qtmmsg.ParseUniqID(p)
			if err != nil {
				return err
			}
			c.uniqid = u
			return nil
		},
		onError: func(rc qtmmsg.ResponseClass) bool { return true },
	}
	c.reqs = append(c.reqs, uniqid)
	// The active port's CFGPROT query needs the port index from the
	// CFGUART response; it is generated lazily by promote().
}

// ppsQuery is the legacy PPS query, used directly on pre-PPS2
// firmware and as the fallback when the PPS2 query is refused.
func (c *Configurator) ppsQuery() *request {
	return c.queryReq("PQTMCFGPPS,R,1", "PQTMCFGPPS",
		func(m qtmmsg.CfgMsg) { c.found.pps = m.(*qtmmsg.CfgPPS) })
}

// generateSpeedChange appends the baud-rate change when the target
// asks for a rate different from the as-found one. The current-port
// CFGUART write (Index 0, baud only) carries the host speed change,
// and its OK goes out at the new rate (the port switches ~2 ms after
// the request), so it is usually destroyed by the switch. The request
// keeps awaiting and succeeds by whichever comes first: its own OK
// when the host catches it, an incidental valid packet at the new rate
// (MaybeSpeedChangeSucceeded), or the OK elicited by a repeat of the
// write after speedChangeRepeatDelay (see GenerateRequests). Ordered
// before the NVM phase so a save persists the new rate and any restart
// goes out at the new speed.
func (c *Configurator) generateSpeedChange() {
	baud, ok := c.target.Props.GetBaudRate()
	if !ok || baud == 0 || c.found.uart == nil || baud == c.found.uart.BaudRate {
		return
	}
	payload, err := qtmmsg.EncodeWrite(&qtmmsg.CfgUART{BaudRate: baud})
	if err != nil {
		panic(err)
	}
	c.speedReq = &request{
		phase:    phaseSpeed,
		payload:  payload,
		sentence: "PQTMCFGUART",
		speed:    int(baud),
		onOK:     func() { c.found.uart.BaudRate = baud },
	}
	c.reqs = append(c.reqs, c.speedReq)
}

// promote readies NotReady requests whose sentence name has no
// earlier live request (single-flight per name), and lazily generates
// requests that depend on earlier answers. noReply requests bypass
// single-flight: they consume no responses, and the speed-change
// repeat must go out while the original is still live (any response
// it elicits belongs to whoever awaits it).
func (c *Configurator) promote() {
	live := make(map[string]bool)
	for _, r := range c.reqs {
		switch r.state {
		case gpsprot.ConfigRequestNotReady:
			if r.phase <= c.phase && (r.noReply || !live[r.sentence]) {
				r.state = gpsprot.ConfigRequestReadyToSend
				live[r.sentence] = true
			}
		case gpsprot.ConfigRequestReadyToSend, gpsprot.ConfigRequestAwaitingResponse,
			gpsprot.ConfigRequestMaybeComplete, gpsprot.ConfigRequestMayResend,
			gpsprot.ConfigRequestPausing:
			live[r.sentence] = true
		}
	}
}

func (c *Configurator) allFinal() bool {
	for _, r := range c.reqs {
		switch r.state {
		case gpsprot.ConfigRequestSucceeded, gpsprot.ConfigRequestFailed, gpsprot.ConfigRequestSkipped:
		default:
			return false
		}
	}
	return len(c.reqs) > 0
}

// response routes a classified PQTM response to the oldest live
// request with the same sentence name.
func (c *Configurator) response(rc qtmmsg.ResponseClass, payload string) error {
	for _, r := range c.reqs {
		if r.sentence != rc.Sentence {
			continue
		}
		switch r.state {
		case gpsprot.ConfigRequestAwaitingResponse, gpsprot.ConfigRequestMaybeComplete:
		default:
			continue
		}
		return r.respond(rc, payload)
	}
	return nil
}

func (r *request) respond(rc qtmmsg.ResponseClass, payload string) error {
	switch rc.Kind {
	case qtmmsg.ResponseOK:
		if r.onOK != nil {
			r.onOK()
		}
		r.state = gpsprot.ConfigRequestSucceeded
	case qtmmsg.ResponseOKData:
		if r.onOKData != nil {
			if err := r.onOKData(payload); err != nil {
				r.err = err
				r.state = gpsprot.ConfigRequestFailed
				return nil
			}
		}
		r.state = gpsprot.ConfigRequestSucceeded
	case qtmmsg.ResponseError:
		if r.onError != nil && r.onError(rc) {
			r.state = gpsprot.ConfigRequestSucceeded
		} else {
			r.err = fmt.Errorf("%s: %s", rc.Sentence, rc.Error)
			r.state = gpsprot.ConfigRequestFailed
		}
	}
	return nil
}

// GetRequestCount returns the number of requests and completeness.
func (c *Configurator) GetRequestCount() (int, bool) {
	return len(c.reqs), c.complete
}

// Request returns the request at the given index.
func (c *Configurator) Request(index int) gpsprot.ConfigRequest {
	return c.reqs[index]
}

// ReceiverInfo returns static information from the probe's PQTMVERNO
// reply and the PQTMUNIQID query.
func (c *Configurator) ReceiverInfo() *gpsprot.ReceiverInfo {
	info := &gpsprot.ReceiverInfo{Vendor: Vendor, SupportedGNSS: signalUniverse().GNSSSet()}
	if c.verno != nil {
		info.Firmware = c.verno.VerStr + " " + c.verno.BuildDate
		info.Hardware = hardwareName(c.verno.VerStr)
		info.VendorSpecific = c.verno
	}
	return info
}

// hardwareName extracts the module name from the version string:
// LG290P03AANR02A01S is an LG290P03. Unrecognized strings are
// returned whole.
func hardwareName(verStr string) string {
	if i := strings.Index(verStr, "AANR"); i > 0 {
		return verStr[:i]
	}
	return verStr
}

// ConfigSupport returns the configuration options this backend supports.
func (c *Configurator) ConfigSupport() gpsprot.ConfigSupportFlags {
	return configSupport
}

// ConfigProps returns the achieved configuration, converted from the
// as-found tuples (updated by set acknowledgements as features land).
func (c *Configurator) ConfigProps() *gpsprot.ConfigProps {
	props := &gpsprot.ConfigProps{}
	c.found.convertToProps(props)
	return props
}

// savesAndResets reports whether the target both saves the
// configuration to NVM and performs a reset that reloads it - the
// gate for the stored-only settings (signals, positioning mode, RTCM
// MSM type), so that persistence and the boot outage are explicitly
// user-requested. A factory reset does not qualify: it restores NVM
// defaults, discarding the save.
func (c *Configurator) savesAndResets() bool {
	o := &c.target.Opts
	return o.Save != gpsprot.SaveNone && (o.Reset == gpsprot.ResetReload || o.Reset == gpsprot.ResetCold)
}

// ConfigRequest interface implementation on request.

// GetPacket returns the framed NMEA packet for this request.
func (r *request) GetPacket() []byte {
	return nmeaPacket(r.payload)
}

// GetSpeedChangeAfter returns the baud rate to switch to after
// sending, 0 for none.
func (r *request) GetSpeedChangeAfter() int {
	return r.speed
}

// GetState returns the request's current state.
func (r *request) GetState() gpsprot.ConfigRequestState {
	return r.state
}

// GetDeadline returns the response deadline. A speed change's first
// deadline is the repeat delay; after the repeat is due it gets the
// full response timeout, still relative to the original send.
func (r *request) GetDeadline() time.Time {
	if r.speed != 0 && !r.repeatDue {
		return r.tBase.Add(speedChangeRepeatDelay)
	}
	return r.tBase.Add(responseTimeout)
}

// GetError returns the failure error.
func (r *request) GetError() error {
	return r.err
}

// SetSentTime records the transmit time. A noReply request (reset or
// speed-change repeat) succeeds when sent; everything else awaits its
// response.
func (r *request) SetSentTime(tSent time.Time) {
	r.tBase = tSent
	if r.noReply {
		r.state = gpsprot.ConfigRequestSucceeded
		return
	}
	r.state = gpsprot.ConfigRequestAwaitingResponse
}

// SetDeadlinePassed times the request out: a speed change whose
// repeat delay passed asks for the repeat and keeps awaiting,
// anything else may be resent.
func (r *request) SetDeadlinePassed() {
	if r.speed != 0 && !r.repeatDue {
		r.repeatDue = true
		return
	}
	r.state = gpsprot.ConfigRequestMayResend
}

// SetWontResend gives up on the request.
func (r *request) SetWontResend() {
	r.err = fmt.Errorf("%s: no response", r.sentence)
	r.state = gpsprot.ConfigRequestFailed
}

// MaybeSpeedChangeSucceeded confirms a speed change from incidental
// traffic: once the host has switched rates, a valid packet read
// speedChangeDelay after the send can only have arrived at the new
// rate.
func (r *request) MaybeSpeedChangeSucceeded(validPacketTime time.Time) {
	if r.speed == 0 || r.state != gpsprot.ConfigRequestAwaitingResponse {
		return
	}
	if validPacketTime.Sub(r.tBase) > speedChangeDelay {
		if r.onOK != nil {
			r.onOK()
		}
		r.state = gpsprot.ConfigRequestSucceeded
	}
}
