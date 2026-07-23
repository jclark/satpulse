package casic

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/casbin"
)

// maxResponseDelay is how long one attempt waits for the ACK/NAK of a
// request. On a healthy line CASIC receivers answer within tens of
// milliseconds. A V5 at 9600 with full NMEA output saturates its line
// and queues responses for seconds; patience for that comes from the
// director's retries, not from a long per-attempt wait - a late ACK
// still matches the retried request, and a healthy line fails fast.
const maxResponseDelay = 2 * time.Second

// speedChangeDelay is how long after sending a baud change until a
// valid packet counts as confirmation. The host switches speed right
// after the write, so any later valid packet must have arrived at the
// new speed; the delay only excludes packets buffered before the
// switch.
const speedChangeDelay = 150 * time.Millisecond

// verDataWait is how long the MON-VER poll waits for its data message
// after the CFG-MSG ACK. The ACK comes from the fast CFG lane, but
// the data is a one-shot output emitted at the receiver's next 1 Hz
// output tick (see the reply-timing record in
// plan/archive/casic-config.md), so it can trail the ACK by up to a
// second; data before the ACK just means the poll landed right
// before a tick. The window covers a full tick plus margin before
// treating the readback as absent.
const verDataWait = 1500 * time.Millisecond

// Configurator implements gpsprot.Configurator for CASIC receivers.
//
// Requests are generated in one batch, created notReady and promoted
// to ready as they become sendable (see promote). The protocol serializes
// them: casic2.md 2.5 and zkw3.md 3.5 forbid sending a second CFG message
// before the receiver replies to the first, so CFG requests go out one at
// a time, in order. Non-CFG requests are serialized only against others
// sharing their class+id: a poll's data response is correlated only by
// the echoed class+id, so two outstanding polls with the same id would
// be ambiguous. Text requests run concurrently; their replies are
// matched by key (see promote).
type Configurator struct {
	target      *gpsprot.ConfigTarget
	rate        *casbin.CfgRate // CFG-RATE readback from the probe
	ver         *casbin.MonVer  // V6 MON-VER readback; nil until answered (never on V5)
	family      fwFamily
	reqs        []*casReq
	phase       int                      // index into genPhases of the next phase to generate
	touched     casbin.CfgCfgSectionMask // configuration sections touched by set requests
	msgEnabled  bool                     // a message-output request enabled some output
	survey      bool                     // this invocation put the receiver into survey-in mode
	tp          *casbin.CfgTP            // latest CFG-TP readback; nil if never answered
	tm5         *casbin.CfgTMode         // latest V5 CFG-TMODE readback
	tm6         *casbin.CfgTMode2        // latest V6 CFG-TMODE2 readback
	navx        *casbin.CfgNavx          // latest V5 CFG-NAVX readback
	navBand     *casbin.CfgNavBand       // latest V6 CFG-NAVBAND readback
	ports       []casbin.CfgPrt          // CFG-PRT readback, one entry per port
	speedReq    *casReq                  // the baud change request, when one was generated
	saveReq     *casReq                  // the save request, when one was generated (gates the reset)
	verReq      *casReq                  // the V6 MON-VER poll, when one was generated
	familyPolls int                      // CFG-RATE re-polls issued by the family phase
	navLimit    *casbin.CfgNavLimit      // latest V6 CFG-NAVLIMIT readback
	pcasSW      string                   // V5 firmware version from PCAS06 query
	pcasHW      string                   // V5 hardware info from PCAS06 query
}

var _ gpsprot.Configurator = (*Configurator)(nil)

// casReqState is the internal request state; see mapping in GetState.
type casReqState int

const (
	reqNotReady casReqState = iota
	reqReady
	reqAwaitingAck
	reqMaybeComplete // ACKed MON-VER poll waiting for its trailing data
	reqMayResend
	reqSucceeded
	reqFailed
)

// casReq is a single configuration request: one CASIC packet expecting
// an ACK or NAK correlated by class+id. When nakOK is set a NAK is an
// acceptable outcome: the request succeeds and whatever it would have
// delivered shows as absence. If a refusal should ever instead trigger
// a fallback request (try message B when A is refused), give casReq an
// onNak callback invoked from handleAck's nakOK branch before it marks
// the request succeeded, and generate the replacement request there;
// promote sends it in order. The TIM-TP to TIM2-TPX fallback worked
// that way until V6 stopped attempting TIM-TP at all.
type casReq struct {
	state      casReqState
	mid        casbin.MsgID // class+id, for ACK correlation
	packet     []byte
	tBase      time.Time // time request was sent
	err        error
	nakOK      bool              // NAK is acceptable, not a failure
	onAck      func()            // records the accepted values on ACK
	noAck      bool              // no response expected (CFG-RST): sending is success
	onData     func(casbin.Msg)  // receives data responses (polls); ACK still completes
	onText     func(string) bool // matches NMEA replies; true completes the request
	optional   bool              // a timed-out request succeeds rather than fails
	verPoll    bool              // MON-VER poll: completes on the data message, not the ACK
	speedAfter int               // new baud rate to switch to after sending
}

// final reports whether the request is in a terminal state.
func (req *casReq) final() bool {
	return req.state == reqSucceeded || req.state == reqFailed
}

// isIdentity reports whether this is an identity query - the MON-VER
// poll or a PCAS06 text request - exempt from phase gating (see
// genPhases). Derived rather than stored so it cannot fall out of
// sync with the request kinds.
func (req *casReq) isIdentity() bool {
	return req.verPoll || req.onText != nil
}

var _ gpsprot.ConfigRequest = (*casReq)(nil)

// familyOf selects the firmware family from a CFG-RATE readback:
// byte 2 is fixRateHz on V6, a rate with no zero value, and
// reserved-as-zero on V5.
func familyOf(rate *casbin.CfgRate) fwFamily {
	if rate.FixRateHz != 0 {
		return familyV6
	}
	return familyV5
}

// newConfigurator creates a Configurator. rate is the probe's
// CFG-RATE readback; nil means it was corrupted on the line and the
// family phase must re-poll it before anything else can be generated.
func newConfigurator(target *gpsprot.ConfigTarget, rate *casbin.CfgRate) *Configurator {
	c := &Configurator{target: target, rate: rate}
	if rate != nil {
		c.family = familyOf(rate)
	}
	return c
}

// ReceiverInfo returns static information about the GPS receiver,
// from the identity queries: MON-VER on V6, PCAS06 text queries on
// V5. Fields stay empty if the receiver never answered. MON-VER's
// SwVersion carries a literal "SW=" prefix on the wire, stripped here
// so the field holds just the value.
func (c *Configurator) ReceiverInfo() *gpsprot.ReceiverInfo {
	info := &gpsprot.ReceiverInfo{Vendor: Vendor, SupportedGNSS: c.supportedGNSS()}
	if c.ver != nil {
		info.Firmware = strings.TrimPrefix(c.ver.SwVersion.String(), "SW=")
		info.Hardware = c.ver.HwVersion.String()
		info.VendorSpecific = c.ver
		return info
	}
	info.Firmware = c.pcasSW
	info.Hardware = c.pcasHW
	return info
}

// supportedGNSS returns the constellations the firmware family can use.
// V5 supports GPS/BDS/GLONASS only; the V6 supported set should be
// refined from CFG-NAVBAND query results once implemented.
func (c *Configurator) supportedGNSS() gpsprot.GNSSSet {
	set := gpsprot.GNSSSetOf(gpsprot.GPS, gpsprot.BDS, gpsprot.GLO)
	if c.family == familyV6 {
		set |= gpsprot.GNSSSetOf(gpsprot.GAL, gpsprot.QZSS, gpsprot.SBAS, gpsprot.NAVIC)
	}
	return set
}

// ConfigSupport returns the configuration options this implementation
// supports.
func (c *Configurator) ConfigSupport() gpsprot.ConfigSupportFlags {
	flags := gpsprot.ConfigSupportSpeed |
		gpsprot.ConfigSupportSurvey | gpsprot.ConfigSupportSurveyAcc |
		gpsprot.ConfigSupportFixedPos | gpsprot.ConfigSupportFixedPosAcc
	if c.family == familyV6 {
		// RTCM MSM output is not declared: no known CASIC firmware
		// emits RTCM (the F8N acknowledges CFG-RTCM but never emits,
		// the -6T NAKs it outright), and flag truthfulness requires
		// not declaring what no receiver delivers. An RTCM output
		// request is accordingly ignored, as raw output is on V5.
		flags |= gpsprot.ConfigSupportSignal | gpsprot.ConfigSupportRaw |
			gpsprot.ConfigSupportSurveyMsg
		// The emission-dependent flags follow the product class
		// encoded in the module name, as u-blox encodes it in model
		// letters: navigation units acknowledge the raw and
		// survey-progress enables but never emit the messages
		// (measured on the F8N), the positioning class emits raw but
		// not survey progress (measured on the AT372-6P), and the
		// timing class delivers everything (measured on the AT362-6T).
		// An unrecognized name - or an unanswered MON-VER query -
		// keeps the family flags.
		hw := ""
		if c.ver != nil {
			hw = c.ver.HwVersion.String()
		}
		switch receiverClass(hw) {
		case "N":
			flags &^= gpsprot.ConfigSupportRaw | gpsprot.ConfigSupportSurveyMsg
		case "P":
			flags &^= gpsprot.ConfigSupportSurveyMsg
		}
	} else {
		flags |= gpsprot.ConfigSupportReload
	}
	return flags
}

// classRe matches a module-name segment carrying the product class:
// an optional F for dual band, the generation digit, the class
// letters (N navigation, P positioning/RTK, T timing, TS secure
// timing, F frequency), then optional constellation digits.
var classRe = regexp.MustCompile(`^F?[0-9](N|P|TS|T|F)[0-9]*$`)

// receiverClass extracts the product class letters from a CASIC
// module name such as AT372-AT6668-6P-34 or ATGM332D-5N71: the first
// segment after the module name (segments separated by hyphens or
// commas) that matches the class pattern. Returns "" when no segment
// matches (including the V5 units' bare chip names).
func receiverClass(hw string) string {
	fields := strings.FieldsFunc(hw, func(r rune) bool { return r == '-' || r == ',' })
	for i := 1; i < len(fields); i++ {
		if m := classRe.FindStringSubmatch(fields[i]); m != nil {
			return m[1]
		}
	}
	return ""
}

// ConfigProps returns the current configuration of the GPS receiver,
// from the readbacks gathered while configuring. Properties that were
// never read back (unsupported or not involved) are absent.
func (c *Configurator) ConfigProps() *gpsprot.ConfigProps {
	props := &gpsprot.ConfigProps{}
	c.tpConfigProps(props)
	c.tmodeConfigProps(props)
	c.signalConfigProps(props)
	c.minElevConfigProps(props)
	if c.speedReq != nil {
		if c.speedReq.state == reqSucceeded {
			props.SetBaudRate(uint32(c.speedReq.speedAfter))
		}
	} else if c.target.UsesAny(gpsprot.PropIDBaudRate) {
		// --show-port: report the wired UART's baud (the speed the host
		// is communicating at) from the CFG-PRT readback. This also
		// reports the authoritative queried value when a requested speed
		// change was suppressed as a no-op. CASIC cannot identify which
		// port is the active one, so the port name itself is left unset
		// (ConfigSupportPort is not advertised).
		if base, ok := c.basePort(); ok {
			props.SetBaudRate(base.BaudRate)
		}
	}
	return props
}

// genPhases are the request generation phases, each gated on all
// earlier non-identity requests being final. The family phase runs
// first and usually generates nothing (the probe's readback selected
// the family already); when the readback was lost it re-polls it, and
// its failure ends the invocation (see GenerateRequests). After it:
// property sets need the
// query phase's readback (read-modify-write), message enabling comes
// after the property work because enabling NMEA output can saturate a
// 9600 line and delay every later acknowledgement, the rate phase
// comes after message enabling so that it fires only when some enable
// was accepted (whether an enable was ACKed is known only once the
// message phase is final) and precedes the baud change and save
// phases so a requested save persists the forced positioning rate,
// the baud change comes after all the work whose acknowledgements it
// would endanger, the save phase persists everything - NAK-driven
// fallback outcomes and the new baud rate alike (a save runs on the
// confirmed line at the new speed, as in the UBX configurator) - and
// the reset phase comes last, after the save is final, both because
// the spec forbids a second CFG message in flight before the first is
// answered (the reset must not be sent while the save awaits its ACK)
// and because a failed save must gate the reset: a reset never
// discards running changes its paired save failed to persist.
// Acknowledged sets record their accepted values as the assumed
// configuration, so no re-polling is needed.
//
// Identity requests (the version/hardware queries) are exempt from
// the gating: on V6 the MON-VER data message arrives only at the
// receiver's next 1 Hz output tick, and nothing but the message phase
// consumes it - its HwVersion drives the class-based support flags -
// so only that phase (needsVer) waits for the poll to resolve, and
// the rest of configuration overlaps the wait. The reset phase is the
// other exception (needsIdentity): a reset restarts the receiver,
// which would discard replies still in flight, so it waits for every
// identity request to resolve - bounded, since identity requests are
// optional and always reach a final state. Run completion also
// requires identity requests to be final (see GetRequestCount), so a
// bare --show-receiver run waits for the readback.
var genPhases = []struct {
	gen           func(*Configurator)
	needsVer      bool
	needsIdentity bool
}{
	{gen: (*Configurator).generateFamilyReqs},
	{gen: (*Configurator).generateQueryReqs},
	{gen: (*Configurator).generateSetReqs},
	{gen: (*Configurator).generateMsgReqs, needsVer: true},
	{gen: (*Configurator).generateRateReqs},
	{gen: (*Configurator).generateSpeedReqs},
	{gen: (*Configurator).generateSaveReqs},
	{gen: (*Configurator).generateResetReqs, needsIdentity: true},
}

// GenerateRequests generates configuration requests and promotes
// requests that have become unambiguous to send.
func (c *Configurator) GenerateRequests() error {
	for c.phase < len(genPhases) && c.configFinal() {
		if c.phase > 0 && c.rate == nil {
			// The family poll resolved without delivering a readback:
			// ACKed with its data message corrupted again. Re-poll a
			// bounded number of times (appended here, since only
			// GenerateRequests may grow the request slice); after
			// that, or on a refusal or timeout, end the invocation -
			// every later phase branches on the family, so generating
			// more would be guessing.
			if !c.anyFailed() && c.familyPolls < maxFamilyPolls {
				c.addFamilyPoll()
				break
			}
			if !c.anyFailed() {
				c.addFailedReq(fmt.Errorf("no CFG-RATE readback to determine the firmware family"))
			}
			c.phase = len(genPhases)
			break
		}
		ph := genPhases[c.phase]
		if ph.needsVer && !c.verFinal() {
			break
		}
		if ph.needsIdentity && !c.identityFinal() {
			break
		}
		ph.gen(c)
		c.phase++
	}
	c.promote()
	return nil
}

// generateFamilyReqs re-polls CFG-RATE when the probe's readback was
// corrupted (probing completed on the poll's ACK alone). Every later
// phase branches on the family, so nothing else generates until the
// readback is in hand; this is the one request whose failure aborts
// the invocation, and an ACK whose data message was corrupted again
// re-polls a bounded number of times (both in GenerateRequests).
func (c *Configurator) generateFamilyReqs() {
	if c.rate == nil {
		c.addFamilyPoll()
	}
}

// maxFamilyPolls bounds the re-polls when ACKs keep arriving without
// their data messages.
const maxFamilyPolls = 3

func (c *Configurator) addFamilyPoll() {
	c.familyPolls++
	pkt, _ := casbin.PackMsg(casbin.CfgRateID, nil)
	c.add(&casReq{mid: casbin.CfgRateID, packet: pkt,
		onData: func(m casbin.Msg) {
			if r, ok := m.(*casbin.CfgRate); ok {
				c.rate = r
				c.family = familyOf(r)
			}
		}})
}

func (c *Configurator) allFinal() bool {
	for _, req := range c.reqs {
		if !req.final() {
			return false
		}
	}
	return true
}

// configFinal reports whether every non-identity request is final;
// identity requests never gate phase advancement (see genPhases).
func (c *Configurator) configFinal() bool {
	for _, req := range c.reqs {
		if !req.isIdentity() && !req.final() {
			return false
		}
	}
	return true
}

// verFinal reports whether the MON-VER poll is final. Trivially true
// on V5, which never generates one.
func (c *Configurator) verFinal() bool {
	return c.verReq == nil || c.verReq.final()
}

// identityFinal reports whether every identity request is final.
func (c *Configurator) identityFinal() bool {
	for _, req := range c.reqs {
		if req.isIdentity() && !req.final() {
			return false
		}
	}
	return true
}

func (c *Configurator) anyFailed() bool {
	for _, req := range c.reqs {
		if req.state == reqFailed {
			return true
		}
	}
	return false
}

// generateQueryReqs polls the messages whose current values the set
// phase needs (read-modify-write) or the target asks to read.
func (c *Configurator) generateQueryReqs() {
	c.generateVerQuery()
	c.generateTPQuery()
	c.generateTModeQuery()
	c.generateSignalQuery()
	c.generateMinElevQuery()
	if c.target.UsesAny(gpsprot.PropIDBaudRate) {
		c.addPollReq(casbin.CfgPrtID, func(m casbin.Msg) {
			if prt, ok := m.(*casbin.CfgPrt); ok {
				c.ports = append(c.ports, *prt)
			}
		})
	}
}

// generateSetReqs generates the property set requests, computed from
// the query phase's readbacks.
func (c *Configurator) generateSetReqs() {
	c.generateTPSet()
	c.generateTModeSet()
	c.generateSignalSet()
	c.generateMinElevSet()
}

// generateSaveReqs generates the save-to-NVM request, retaining its
// pointer so the reset phase can gate on the save's outcome. NVM is
// written only by an invocation in which everything succeeded: after
// any failed request no save is attempted - the failure is the
// invocation's reported error, and NVM keeps its last saved state.
func (c *Configurator) generateSaveReqs() {
	if c.anyFailed() {
		return
	}
	switch c.target.Opts.Save {
	case gpsprot.SaveAll:
		c.saveReq = c.addMsg(&casbin.CfgCfg{Mask: c.saveMask(casbin.CfgCfgSectionAll), OpMode: casbin.CfgCfgOpSave}, casReq{})
	case gpsprot.SaveMinimal:
		touched := c.touched
		if c.speedReq != nil && c.speedReq.state == reqSucceeded {
			// A speed change confirmed by the CFG-RATE poll or by
			// traffic at the new rate never runs onAck, so its port
			// section is accounted for here.
			touched |= casbin.CfgCfgSectionPort
		}
		if touched != 0 {
			c.saveReq = c.addMsg(&casbin.CfgCfg{Mask: c.saveMask(touched), OpMode: casbin.CfgCfgOpSave}, casReq{})
		}
	}
}

// generateResetReqs generates the reset request. It runs as the final
// phase, gated on the save being final (see genPhases), so the reset
// goes out only after the save's ACK/NAK has arrived. If a save was
// generated but did not succeed, no reset is generated at all: the
// save's failure is the invocation's reported error, and a reset must
// never discard running changes its paired save failed to persist. A
// reset with no save (none requested, or a minimal save with nothing
// touched) proceeds ungated. A save skipped because a request failed
// (see generateSaveReqs) gates its paired reset the same way as a
// failed save; a reset with no save requested proceeds even after
// failures, since it discards running changes by design.
func (c *Configurator) generateResetReqs() {
	if c.target.Opts.Save != gpsprot.SaveNone && c.anyFailed() {
		return
	}
	if c.saveReq != nil && c.saveReq.state != reqSucceeded {
		return
	}
	switch c.target.Opts.Reset {
	case gpsprot.ResetReload:
		if c.family == familyV6 {
			return
		}
		c.addReq(&casbin.CfgCfg{Mask: c.saveMask(casbin.CfgCfgSectionAll), OpMode: casbin.CfgCfgOpLoad})
	case gpsprot.ResetCold:
		c.addRstReq(casbin.CfgRstStartCold)
	case gpsprot.ResetFactory:
		c.addRstReq(casbin.CfgRstStartFactory)
	}
}

// saveMask returns the CFG-CFG section mask to send. V5 firmware
// honours the section bits. V6 documents the field as reserved but
// does NOT ignore it: mask 0 is ACKed and saves nothing (observed on
// the F8N), so V6 always gets the all-sections mask - its save
// granularity is a single group.
func (c *Configurator) saveMask(mask casbin.CfgCfgSectionMask) casbin.CfgCfgSectionMask {
	if c.family == familyV6 {
		return casbin.CfgCfgSectionAll
	}
	return mask
}

// bbrReset clears everything learned from satellites plus saved
// position and config, but keeps clock drift and oscillator parameters
// (learned locally, not from satellites). Matches the casictool
// reference. V6 firmware's verified reset command clears no BBR
// sections; its start mode implies the scope.
const bbrReset = casbin.CfgRstNavBbrEphemeris | casbin.CfgRstNavBbrAlmanac |
	casbin.CfgRstNavBbrHealth | casbin.CfgRstNavBbrIonosphere |
	casbin.CfgRstNavBbrPosition | casbin.CfgRstNavBbrUTCParams |
	casbin.CfgRstNavBbrRTC | casbin.CfgRstNavBbrConfig

// addRstReq appends a CFG-RST request. The receiver restarts without
// acknowledging, so the request succeeds when sent.
func (c *Configurator) addRstReq(startMode casbin.CfgRstStartMode) {
	bbr := casbin.CfgRstNavBbrV6Reserved
	resetMode := casbin.CfgRstResetModeV6Reserved
	if c.family == familyV5 {
		bbr = bbrReset
		resetMode = casbin.CfgRstResetHardwareImmediate
	}
	m := &casbin.CfgRst{NavBbrMask: bbr, ResetMode: resetMode, StartMode: startMode}
	c.addMsg(m, casReq{noAck: true})
}

// generateSpeedReqs generates the baud rate change. The set uses port
// id CfgPrtPortCurrent with the other port settings preserved
// from the readback. The receiver switches speed immediately and sends
// its ACK at the new rate, where the switch can garble it, so the
// request is followed by a CFG-RATE poll sent after the host switched:
// the poll is retried like any request, and because it is solicited at
// the new rate, its answer confirms the speed change conclusively and
// immediately (incidental traffic confirms too, but only after the
// exclusion delay that rules out packets buffered from before the
// switch). Only when the change is confirmed does the NVM phase
// proceed, so a requested save persists the new rate.
func (c *Configurator) generateSpeedReqs() {
	baud, ok := c.target.Props.GetBaudRate()
	if !ok {
		return
	}
	base, haveBase := c.basePort()
	if !haveBase {
		return
	}
	if base.BaudRate == baud {
		c.touchNoOp(casbin.CfgCfgSectionPort)
		return
	}
	m := &casbin.CfgPrt{PortID: casbin.CfgPrtPortCurrent, ProtoMask: base.ProtoMask, Mode: base.Mode, BaudRate: baud}
	c.speedReq = c.addMsg(m, casReq{speedAfter: int(baud)})
	c.addPollReq(casbin.CfgRateID, func(casbin.Msg) {
		if c.speedReq.state == reqAwaitingAck {
			c.speedReq.state = reqSucceeded
		}
	})
}

// basePort returns the polled port entry whose settings a port write
// preserves: CfgPrtPortUART0 (the wired UART on all known boards), or the
// first entry reported.
func (c *Configurator) basePort() (casbin.CfgPrt, bool) {
	if len(c.ports) == 0 {
		return casbin.CfgPrt{}, false
	}
	base := c.ports[0]
	for _, p := range c.ports {
		if p.PortID == casbin.CfgPrtPortUART0 {
			base = p
		}
	}
	return base, true
}

// promote readies notReady requests that can be sent now. The primary
// rule is the spec's one-CFG-at-a-time requirement: casic2.md 2.5 and
// zkw3.md 3.5 both state the sender must not send a second CFG message
// before the receiver replies to the received CFG message, so at most
// one CFG-class request is ever live (ready or outstanding) at a time
// and CFG requests go out serially in order. The ACK is the reply: an
// ACKed MON-VER poll still waiting for its tick-scheduled data message
// (reqMaybeComplete) does not hold the CFG slot - the receiver
// services further CFG requests while a one-shot output is pending
// (measured; see the reply-timing record in
// plan/archive/casic-config.md). The class+id map
// still serializes non-CFG requests among themselves: a poll's data
// response is correlated only by the echoed class+id. Text requests
// are exempt from all of it and promote immediately: their replies
// are matched by key, so concurrent text requests with distinct keys
// (the only kind generated) are unambiguous, and V5 answers each
// query at once.
//
// The speed change is carved out: a sent baud change switches the
// receiver's rate immediately, so its ACK arrives at the new speed
// where the switch can garble it, and the CFG-RATE confirmation poll
// (itself a CFG message) must be transmitted at the new rate to resolve
// the change conclusively. An awaiting-ack speed request therefore does
// not count as live; without this carve-out the confirmation poll can
// never be promoted and the speed change deadlocks into timeout.
func (c *Configurator) promote() {
	live := make(map[casbin.MsgID]bool)
	cfgLive := false
	for _, req := range c.reqs {
		if req.onText != nil {
			if req.state == reqNotReady {
				req.state = reqReady
			}
			continue
		}
		isCfg := req.mid.CfgClass()
		switch req.state {
		case reqNotReady:
			if !live[req.mid] && !(isCfg && cfgLive) {
				req.state = reqReady
			}
			live[req.mid] = true
		case reqReady, reqAwaitingAck, reqMaybeComplete, reqMayResend:
			live[req.mid] = true
		}
		if isCfg {
			switch req.state {
			case reqNotReady, reqReady, reqMayResend:
				cfgLive = true
			case reqAwaitingAck:
				if req.speedAfter == 0 {
					cfgLive = true
				}
			}
		}
	}
}

// GetRequestCount returns the current number of requests and whether
// the slice is complete. The slice is complete only when every request
// is final: a NAK on a live request may still generate a fallback
// request, and later phases generate more requests.
func (c *Configurator) GetRequestCount() (int, bool) {
	return len(c.reqs), c.phase == len(genPhases) && c.allFinal()
}

// Request returns the ConfigRequest at the given index.
func (c *Configurator) Request(index int) gpsprot.ConfigRequest {
	return c.reqs[index]
}

// add appends a request, to be sent once no earlier request with the
// same class+id is outstanding (see promote). All requests enter the
// slice through here.
func (c *Configurator) add(req *casReq) *casReq {
	req.state = reqNotReady
	c.reqs = append(c.reqs, req)
	return req
}

// addMsg is add for a request carrying a serialized message. On ACK it
// records the configuration section the message touches (for minimal
// saves) and whether it enabled output; recording on ACK rather than at
// generation means only accepted changes count, so a minimal save is
// not generated when every change was refused.
func (c *Configurator) addMsg(m casbin.Msg, req casReq) *casReq {
	if section, enables := setSection(m), msgEnablesOutput(m); section != 0 || enables {
		prev := req.onAck
		req.onAck = func() {
			c.touched |= section
			if enables {
				c.msgEnabled = true
			}
			if prev != nil {
				prev()
			}
		}
	}
	req.mid = m.ID()
	req.packet = serialize(m)
	return c.add(&req)
}

// addReq appends a request for the given message. A NAK fails it.
func (c *Configurator) addReq(m casbin.Msg) {
	c.addMsg(m, casReq{})
}

// addReqNakOK is addReq for requests where a NAK is an acceptable
// outcome.
func (c *Configurator) addReqNakOK(m casbin.Msg) {
	c.addMsg(m, casReq{nakOK: true})
}

// addSetReq appends a property set request. When the receiver
// acknowledges it, onAck records the accepted values as the
// receiver's assumed configuration - per the semantics, the achieved
// value a set reports is what the receiver accepted, and a refusal
// (NAK) leaves the assumed configuration unchanged.
func (c *Configurator) addSetReq(m casbin.Msg, onAck func()) {
	c.addMsg(m, casReq{onAck: onAck})
}

// addPollReq appends an empty-payload poll of the given CFG message.
// The data response is passed to onData; a NAK means the message does
// not exist in this firmware, which is acceptable (shown by absence).
func (c *Configurator) addPollReq(mid casbin.MsgID, onData func(casbin.Msg)) {
	pkt, _ := casbin.PackMsg(mid, nil)
	c.add(&casReq{mid: mid, packet: pkt, nakOK: true, onData: onData})
}

// addFailedReq appends a request that is born failed: generation
// already knows it cannot be realized. Nothing is sent; the director
// reports the error and the invocation continues with the rest.
func (c *Configurator) addFailedReq(err error) {
	c.reqs = append(c.reqs, &casReq{state: reqFailed, err: err})
}

// touchNoOp records the save section of a set suppressed as a no-op.
// The suppression is judged against the running configuration, but a
// requested save persists into NVM, which cannot be read and may
// differ; marking the section makes the save cover the asserted value
// (the save copies the already-correct running state to NVM) without
// putting a redundant write on the wire.
func (c *Configurator) touchNoOp(section casbin.CfgCfgSectionMask) {
	c.touched |= section
}

// addTextReq appends an NMEA text request whose reply is matched by
// onText. There is no acknowledgement and no reply is guaranteed, so
// the request is optional: silence is acceptable. All text requests
// are identity queries, exempt from phase gating (see isIdentity).
func (c *Configurator) addTextReq(sentence string, onText func(string) bool) {
	c.add(&casReq{packet: []byte(sentence), onText: onText, optional: true})
}

// setSection returns the CFG-CFG save-section bit a set request
// touches, for minimal saves. The nonzero result also gates the
// minimal save: a value here, recorded when the request is ACKed
// (see addMsg), is what tells generateSaveReqs an accepted change
// happened and a save is due. The V6-only CFG messages therefore map
// to the V5 section their setting belongs to (time mode, band, and
// nav limit are Nav) even though the particular bit is meaningless on
// V6, where saveMask substitutes the all-sections mask - only being
// nonzero matters, so that the save fires. A CFG-MSG poll (the V6
// MON-VER query) and CFG-RST return 0: neither touches saved
// configuration, so neither may trigger a save.
func setSection(m casbin.Msg) casbin.CfgCfgSectionMask {
	if mt, ok := m.(*casbin.CfgMsg); ok && mt.Rate == casbin.CfgMsgRatePoll {
		return 0
	}
	switch m.ID() {
	case casbin.CfgMsgID:
		return casbin.CfgCfgSectionMsg
	case casbin.CfgPrtID:
		return casbin.CfgCfgSectionPort
	case casbin.CfgTPID:
		return casbin.CfgCfgSectionTP
	case casbin.CfgRateID, casbin.CfgTModeID, casbin.CfgNavxID,
		casbin.CfgTMode2ID, casbin.CfgNavBandID, casbin.CfgNavLimID:
		return casbin.CfgCfgSectionNav
	}
	return 0
}

// msgEnablesOutput reports whether m is a message-output request that
// turns some output on: a CFG-MSG with a nonzero output rate. A poll
// (CfgMsgRatePoll) enables nothing. Such a request's ACK is wrapped in addMsg
// to set msgEnabled, so the flag reflects only accepted enables (many
// enables are nakOK): it gates the CFG-RATE set in the rate phase (see
// generateRateReqs), so that whenever an invocation actually enables
// output that output runs at 1 Hz.
func msgEnablesOutput(m casbin.Msg) bool {
	if mt, ok := m.(*casbin.CfgMsg); ok {
		return mt.Rate != casbin.CfgMsgRateOff && mt.Rate != casbin.CfgMsgRatePoll
	}
	return false
}

func serialize(m casbin.Msg) []byte {
	pkt, err := casbin.Serialize(m)
	if err != nil {
		panic(fmt.Sprintf("serializing %v: %v", m.ID(), err))
	}
	return pkt
}

// nativeMsg processes receiver messages routed from the ConfigProtocol.
// Non-ACK messages are offered to the oldest outstanding poll request
// with the same class+id (a poll's data response echoes the request's
// class+id and precedes its ACK).
func (c *Configurator) nativeMsg(m casbin.Msg, tRead time.Time) error {
	switch mt := m.(type) {
	case *casbin.AckAck:
		c.handleAck(casbin.MakeMsgID(mt.ClsID, mt.MsgID), true, tRead)
	case *casbin.AckNak:
		c.handleAck(casbin.MakeMsgID(mt.ClsID, mt.MsgID), false, tRead)
	case *casbin.MonVer:
		c.ver = mt
		// The data message completes an already-ACKed poll (the
		// ACK-first response order); with data-first, the ACK that
		// follows completes it (see handleAck).
		if c.verReq != nil && c.verReq.state == reqMaybeComplete {
			c.verReq.state = reqSucceeded
		}
	default:
		for _, req := range c.reqs {
			if req.mid == m.ID() && req.state == reqAwaitingAck && req.onData != nil {
				req.onData(m)
				break
			}
		}
	}
	return nil
}

// nativeText offers an NMEA sentence payload to outstanding text
// requests (PCAS06 queries awaiting their GPTXT reply).
func (c *Configurator) nativeText(payload string, tRead time.Time) error {
	for _, req := range c.reqs {
		if req.state == reqAwaitingAck && req.onText != nil && req.onText(payload) {
			req.state = reqSucceeded
			break
		}
	}
	return nil
}

// handleAck resolves an ACK/NAK against the oldest outstanding request
// with the acknowledged class+id. Responses arrive in request order on
// all tested receivers, and promote ensures at most one request per
// class+id is outstanding. After a resend, the earlier send's late ACK
// can complete the resend, leaving its real ACK for the next same-mid
// request; acknowledgements carry only the class+id, so the two cannot
// be told apart.
func (c *Configurator) handleAck(mid casbin.MsgID, ack bool, tRead time.Time) {
	for _, req := range c.reqs {
		if req.mid != mid || req.state != reqAwaitingAck {
			continue
		}
		delay := tRead.Sub(req.tBase)
		if delay < 0 || delay > maxResponseDelay {
			// A response outside the window belongs to an earlier
			// send of this (since resent) request; it must not
			// confirm the resend.
			return
		}
		if req.verPoll && ack {
			// The poll's outcome is its data message. When the data
			// preceded the ACK it is already in hand; otherwise hold
			// the request open for the trailing data (ACK-first
			// firmware) - the idle deadline treats silence as absence.
			if c.ver != nil {
				req.state = reqSucceeded
			} else {
				req.state = reqMaybeComplete
				req.tBase = tRead
			}
			return
		}
		if ack {
			if req.onAck != nil {
				req.onAck()
			}
			req.state = reqSucceeded
		} else if req.nakOK {
			req.state = reqSucceeded
		} else {
			req.state = reqFailed
			req.err = fmt.Errorf("receiver refused %v", mid)
		}
		return
	}
}

func (req *casReq) invalidStateMsg(method string) string {
	return fmt.Sprintf("%s called when state is %v", method, req.state)
}

// GetPacket returns the packet bytes for this request.
func (req *casReq) GetPacket() []byte {
	switch req.state {
	case reqReady, reqMayResend, reqFailed:
		return req.packet
	}
	panic(req.invalidStateMsg("GetPacket"))
}

// GetSpeedChangeAfter returns the baud rate the host must switch to
// after sending this request, or 0.
func (req *casReq) GetSpeedChangeAfter() int {
	switch req.state {
	case reqReady, reqMayResend, reqFailed:
		return req.speedAfter
	}
	panic(req.invalidStateMsg("GetSpeedChangeAfter"))
}

// GetState reports the request state to the director.
func (req *casReq) GetState() gpsprot.ConfigRequestState {
	switch req.state {
	case reqNotReady:
		return gpsprot.ConfigRequestNotReady
	case reqReady:
		return gpsprot.ConfigRequestReadyToSend
	case reqAwaitingAck:
		return gpsprot.ConfigRequestAwaitingResponse
	case reqMaybeComplete:
		return gpsprot.ConfigRequestMaybeComplete
	case reqMayResend:
		return gpsprot.ConfigRequestMayResend
	case reqSucceeded:
		return gpsprot.ConfigRequestSucceeded
	case reqFailed:
		return gpsprot.ConfigRequestFailed
	default:
		panic(fmt.Sprintf("unexpected internal state: %v", req.state))
	}
}

// GetDeadline returns the absolute time by which a response is expected.
func (req *casReq) GetDeadline() time.Time {
	switch req.state {
	case reqAwaitingAck:
		return req.tBase.Add(maxResponseDelay)
	case reqMaybeComplete:
		return req.tBase.Add(verDataWait)
	}
	panic(req.invalidStateMsg("GetDeadline"))
}

// GetError returns why the request failed.
func (req *casReq) GetError() error {
	if req.state != reqFailed {
		panic(req.invalidStateMsg("GetError"))
	}
	return req.err
}

// SetSentTime records when the request packet was transmitted. A
// request that expects no acknowledgement succeeds at this point.
func (req *casReq) SetSentTime(tSent time.Time) {
	switch req.state {
	case reqReady, reqMayResend:
		if req.noAck {
			req.state = reqSucceeded
			return
		}
		req.state = reqAwaitingAck
		req.tBase = tSent
	default:
		panic(req.invalidStateMsg("SetSentTime"))
	}
}

// SetDeadlinePassed marks the response window as expired. An ACKed
// MON-VER poll whose data never arrived succeeds with the readback
// absent.
func (req *casReq) SetDeadlinePassed() {
	switch req.state {
	case reqAwaitingAck:
		req.state = reqMayResend
	case reqMaybeComplete:
		req.state = reqSucceeded
	default:
		panic(req.invalidStateMsg("SetDeadlinePassed"))
	}
}

// SetWontResend gives up on the request: failure, unless the request
// was optional (an unanswered PCAS06 query just leaves info empty).
func (req *casReq) SetWontResend() {
	if req.state != reqMayResend {
		panic(req.invalidStateMsg("SetWontResend"))
	}
	if req.optional {
		req.state = reqSucceeded
		return
	}
	req.state = reqFailed
	if req.err == nil {
		req.err = fmt.Errorf("no response to %v", req.mid)
	}
}

// MaybeSpeedChangeSucceeded confirms a baud change: once the host has
// switched speed, a valid packet read after the exclusion delay can
// only have arrived at the new speed.
func (req *casReq) MaybeSpeedChangeSucceeded(validPacketTime time.Time) {
	if req.state != reqAwaitingAck || req.speedAfter == 0 {
		return
	}
	if validPacketTime.Sub(req.tBase) > speedChangeDelay {
		req.state = reqSucceeded
	}
}
