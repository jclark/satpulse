package septentrio

import (
	"fmt"
	"strings"
	"time"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/sbfbin"
)

// Vendor is the vendor name reported in ReceiverInfo.
const Vendor = "Septentrio"

// configSupport is everything except the survey capabilities - Septentrio
// has no parameterized/terminating/observable survey operation
// ("setPVTMode, Static, , auto" is an auto-computed reference, not a
// survey) - and fixed-position accuracy, which has no command carrier.
// Declaring it as full minus these gains future flags automatically.
const configSupport gpsprot.ConfigSupportFlags = gpsprot.ConfigSupportFull &^
	(gpsprot.ConfigSupportSurvey | gpsprot.ConfigSupportSurveyAcc |
		gpsprot.ConfigSupportSurveyMsg | gpsprot.ConfigSupportFixedPosAcc)

// septSignalNames maps each device-independent signal to its Septentrio
// signal name (the coarse config table; distinct from the conversion layer's
// finer signal-number table). Signals with no entry have no Septentrio
// carrier and are shown as absence. Septentrio names with no entry here
// (GALE5 AltBOC, GLOL2P, QZSL1CB) have no device-independent analogue:
// sets preserve them as found and readbacks ignore them.
var septSignalNames = map[gpsprot.Signal]string{
	gpsprot.SigGPSL1CA:  "GPSL1CA",
	gpsprot.SigGPSL1C:   "GPSL1C",
	gpsprot.SigGPSL2P:   "GPSL2PY",
	gpsprot.SigGPSL2C:   "GPSL2C",
	gpsprot.SigGPSL5:    "GPSL5",
	gpsprot.SigGLOL1:    "GLOL1CA",
	gpsprot.SigGLOL2:    "GLOL2CA",
	gpsprot.SigGLOL3:    "GLOL3",
	gpsprot.SigGALE1:    "GALE1BC",
	gpsprot.SigGALE5a:   "GALE5a",
	gpsprot.SigGALE5b:   "GALE5b",
	gpsprot.SigGALE6:    "GALE6BC",
	gpsprot.SigBDSB1I:   "BDSB1I",
	gpsprot.SigBDSB1C:   "BDSB1C",
	gpsprot.SigBDSB2I:   "BDSB2I",
	gpsprot.SigBDSB2b:   "BDSB2b",
	gpsprot.SigBDSB2a:   "BDSB2a",
	gpsprot.SigBDSB3I:   "BDSB3I",
	gpsprot.SigQZSSL1CA: "QZSL1CA",
	gpsprot.SigQZSSL1C:  "QZSL1C",
	gpsprot.SigQZSSL1S:  "QZSL1S",
	gpsprot.SigQZSSL2C:  "QZSL2C",
	gpsprot.SigQZSSL5:   "QZSL5",
	gpsprot.SigQZSSL6:   "QZSL6",
	gpsprot.SigNAVICL5:  "NAVICL5",
	gpsprot.SigSBASL1CA: "GEOL1",
	gpsprot.SigSBASL5:   "GEOL5",
}

// septSignalFromName is the reverse of septSignalNames.
var septSignalFromName = func() map[string]gpsprot.Signal {
	m := make(map[string]gpsprot.Signal, len(septSignalNames))
	for sig, name := range septSignalNames {
		m[name] = sig
	}
	return m
}()

// signalSetFromNames converts a list of Septentrio signal names to the
// device-independent SignalSet, ignoring names with no gpsprot analogue.
func signalSetFromNames(names []string) gpsprot.SignalSet {
	var ss gpsprot.SignalSet
	for _, name := range names {
		if sig, ok := septSignalFromName[name]; ok {
			ss |= gpsprot.SignalSetOf(sig)
		}
	}
	return ss
}

// grcCmd is the probe command: state-neutral, repeatable, answered by every
// receiver in the family with one ReceiverCapabilities state line.
const grcCmd = "getReceiverCapabilities"

// rxCaps is the parsed getReceiverCapabilities reply: the single source for
// all capability gating (never gate on a hardware model string).
type rxCaps struct {
	signals []string        // supported signal names, verbatim
	ports   []string        // connection descriptors (DSK1, COM1, USB1, ...)
	caps    map[string]bool // enabled capabilities (GalOSNMA, PPPGalileoHAS-SIS, ...)
	sigSet  gpsprot.SignalSet
}

// parseCaps parses the ReceiverCapabilities state line of a grc reply:
//
//	ReceiverCapabilities, Main, <signals>, <ports>, <capabilities>, <measRate>, <pvtRate>
func parseCaps(r *Reply) (*rxCaps, error) {
	for _, s := range r.States {
		fields := strings.Split(s, ",")
		for i, f := range fields {
			fields[i] = strings.TrimSpace(f)
		}
		if fields[0] != "ReceiverCapabilities" || len(fields) < 5 {
			continue
		}
		c := &rxCaps{
			signals: strings.Split(fields[2], "+"),
			ports:   strings.Split(fields[3], "+"),
			caps:    make(map[string]bool),
		}
		for _, name := range strings.Split(fields[4], "+") {
			c.caps[name] = true
		}
		c.sigSet = signalSetFromNames(c.signals)
		return c, nil
	}
	return nil, fmt.Errorf("septentrio: no ReceiverCapabilities state line in grc reply")
}

// ConfigProtocol implements gpsprot.ConfigProtocol for Septentrio receivers.
type ConfigProtocol struct {
	caps    *rxCaps // stored from the grc probe reply
	port    string  // our connection descriptor, from the probe reply's prompt
	rxSetup *sbfbin.ReceiverSetup
	cfg     *Configurator // created during Configure()
}

var _ gpsprot.ConfigProtocol = (*ConfigProtocol)(nil)

// NewConfigProtocol creates a new Septentrio configuration protocol.
func NewConfigProtocol() *ConfigProtocol {
	return &ConfigProtocol{}
}

// ProbePacket returns the grc probe command.
func (cp *ConfigProtocol) ProbePacket() []byte {
	return []byte(grcCmd + "\r\n")
}

// ProbeOK returns true once a grc reply has been parsed.
func (cp *ConfigProtocol) ProbeOK() bool {
	return cp.caps != nil
}

// NativeMsg processes reply messages delivered by the ReplyProcessor and
// ReceiverSetup blocks delivered by the SBF PacketProcessor.
func (cp *ConfigProtocol) NativeMsg(tag gpsprot.Tag, msgID string, msg any, tRead time.Time) error {
	switch m := msg.(type) {
	case *Reply:
		if m.Kind == ReplyAck && m.Echo == grcCmd {
			if caps, err := parseCaps(m); err == nil {
				cp.caps = caps
				cp.port = m.Prompt
			}
		}
		if cp.cfg != nil {
			return cp.cfg.reply(m, tRead)
		}
	case *sbfbin.Block:
		if rs, ok := m.Params.(*sbfbin.ReceiverSetup); ok {
			cp.rxSetup = rs
			if cp.cfg != nil {
				cp.cfg.receiverSetup(rs)
			}
		}
	}
	return nil
}

// Configure creates a Configurator for the given configuration target.
func (cp *ConfigProtocol) Configure(target *gpsprot.ConfigTarget) (gpsprot.Configurator, error) {
	if cp.caps == nil {
		panic("Configure called without successful ProbeOK()")
	}
	cp.cfg = &Configurator{
		caps:    cp.caps,
		target:  target,
		port:    cp.port,
		rxSetup: cp.rxSetup,
		phase:   phaseInit,
	}
	return cp.cfg, nil
}
