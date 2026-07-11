package quectel

import (
	"maps"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/internal/nmea"
	"github.com/jclark/satpulse/gps/lib/geopos"
	"github.com/jclark/satpulse/gps/lib/qtmmsg"
)

// fakeResponses maps a request payload (between $ and *) to the
// response payloads the fake receiver answers with. All response
// strings are verbatim LG290P R02A01S captures unless noted.
var fakeResponses = map[string][]string{
	"PQTMCFGUART,R":     {"PQTMCFGUART,OK,1,460800,8,0,1,0"},
	"PQTMCFGPPS2,R,1":   {"PQTMCFGPPS2,OK,1,1,100,1,1,0,1000,0,1,0,0,0"},
	"PQTMCFGPPS,R,1":    {"PQTMCFGPPS,OK,1,1,100,1,1,0"},
	"PQTMCFGFIXRATE,R":  {"PQTMCFGFIXRATE,OK,1000"},
	"PQTMCFGELETHD,R":   {"PQTMCFGELETHD,OK,5.0"},
	"PQTMCFGCNST,R":     {"PQTMCFGCNST,OK,1,1,1,1,1,1"},
	"PQTMCFGSIGNAL,R":   {"PQTMCFGSIGNAL,OK,07,03,0F,3F,0F,01"},
	"PQTMCFGSVIN,R":     {"PQTMCFGSVIN,OK,0,0,0.0,0.0000,0.0000,0.0000,0.0"},
	"PQTMCFGRCVRMODE,R": {"PQTMCFGRCVRMODE,OK,1"},
	"PQTMCFGRTCM,R":     {"PQTMCFGRTCM,OK,4,0,-90.0,07,06,1,0"},
	"PQTMCFGRSID,R":     {"PQTMCFGRSID,OK,290"},
	"PQTMUNIQID":        {"PQTMUNIQID,OK,8,0000183B31B3C252"},
	"PQTMCFGPROT,R,1,1": {"PQTMCFGPROT,OK,1,1,00000007,00000007"},
}

const vernoPayload = "PQTMVERNO,LG290P03AANR02A01S,2025/12/12,11:21:01"

func feed(t *testing.T, cp *ConfigProtocol, payload string, tm time.Time) {
	t.Helper()
	if err := cp.NativeMsg(nmea.Tag, "", &nmea.Sentence{Payload: payload}, tm); err != nil {
		t.Fatalf("NativeMsg(%q): %v", payload, err)
	}
}

// runConfigTarget drives a ConfigProtocol through probe and Configure
// for the given target, running the real ConfigDirector against the
// fake receiver's response map. It returns the Configurator, the
// director's error count, and the request payloads sent, in order.
func runConfigTarget(t *testing.T, target *gpsprot.ConfigTarget, responses map[string][]string) (*Configurator, int, []string) {
	t.Helper()
	return runConfigTargetSeq(t, target, responses, nil)
}

// runConfigTargetSeq is runConfigTarget with an optional per-payload
// response queue: seq[payload] holds successive response groups,
// consumed one per request of that payload, taking precedence over the
// static map while it lasts. It models a payload whose answer changes
// between sends - the speed change's OK lost in the rate switch (empty
// group) and the OK answering the repeat of the same write.
func runConfigTargetSeq(t *testing.T, target *gpsprot.ConfigTarget, responses map[string][]string, seq map[string][][]string) (*Configurator, int, []string) {
	t.Helper()
	cp := NewConfigProtocol()
	now := time.Date(2026, 7, 7, 0, 0, 0, 0, time.UTC)
	feed(t, cp, vernoPayload, now)
	if !cp.ProbeOK() {
		t.Fatal("ProbeOK false after PQTMVERNO response")
	}
	cfgtor, err := cp.Configure(target)
	if err != nil {
		t.Fatalf("Configure: %v", err)
	}
	var sent []string
	dir := gpsprot.NewConfigDirector(cfgtor, 3)
	dir.AdvanceTimeTo(now)
	for act := range dir.Actions() {
		switch act.Type {
		case gpsprot.ConfigActionSendRequest:
			req := cfgtor.Request(act.Index)
			now = now.Add(time.Millisecond)
			dir.AdvanceTimeTo(now)
			req.SetSentTime(now)
			payload := string(act.Packet)
			payload = payload[1:strings.IndexByte(payload, '*')]
			sent = append(sent, payload)
			resps := responses[payload]
			if q := seq[payload]; len(q) > 0 {
				resps = q[0]
				seq[payload] = q[1:]
			}
			for _, resp := range resps {
				now = now.Add(time.Millisecond)
				dir.AdvanceTimeTo(now)
				feed(t, cp, resp, now)
				dir.ValidPacketReceived(now)
			}
		case gpsprot.ConfigActionWaitUntil:
			now = act.Deadline
			dir.AdvanceTimeTo(now)
		case gpsprot.ConfigActionError:
			t.Logf("config error: %v", act.Error)
		}
	}
	return cfgtor.(*Configurator), dir.ErrorCount, sent
}

func runConfig(t *testing.T, responses map[string][]string) (*Configurator, int) {
	t.Helper()
	c, errCount, _ := runConfigTarget(t, &gpsprot.ConfigTarget{}, responses)
	return c, errCount
}

func TestConfiguratorQueryPhase(t *testing.T) {
	c, errCount := runConfig(t, fakeResponses)
	if errCount != 0 {
		t.Errorf("director errors: %d", errCount)
	}
	expect := asFound{
		uart:     &qtmmsg.CfgUART{Index: 1, BaudRate: 460800, DataBit: 8, StopBit: 1},
		pps2:     &qtmmsg.CfgPPS2{Index: 1, Enable: 1, Duration: 100, Mode: 1, Polarity: 1, Period: 1000, Reserved2: 1},
		fixRate:  &qtmmsg.CfgFixRate{FixInterval: 1000},
		eleThd:   &qtmmsg.CfgEleThd{Ele: 5.0},
		cnst:     &qtmmsg.CfgCnst{GPS: 1, GLONASS: 1, Galileo: 1, BDS: 1, QZSS: 1, NavIC: 1},
		signal:   &qtmmsg.CfgSignal{GPSSig: 0x07, GLOSig: 0x03, GALSig: 0x0F, BDSSig: 0x3F, QZSSig: 0x0F, NACSig: 0x01},
		svin:     &qtmmsg.CfgSvin{},
		rcvrMode: &qtmmsg.CfgRcvrMode{Mode: 1},
		prot:     &qtmmsg.CfgProt{PortType: 1, PortID: 1, InputProt: 0x7, OutputProt: 0x7},
		rtcm:     &qtmmsg.CfgRtcm{MSMType: 4, MSMElevThd: -90.0, Reserved0: "07", Reserved1: "06", EPHMode: 1},
		rsid:     &qtmmsg.CfgRSID{ID: 290},
	}
	if !reflect.DeepEqual(c.found, expect) {
		t.Errorf("as-found state:\ngot  %+v\nwant %+v", c.found, expect)
	}
	if c.found.pps != nil {
		t.Errorf("legacy PPS queried despite PPS2 success: %+v", c.found.pps)
	}
	if c.uniqid == nil || c.uniqid.ID != "0000183B31B3C252" {
		t.Errorf("uniqid = %+v", c.uniqid)
	}
	info := c.ReceiverInfo()
	if info.Vendor != "Quectel" || info.Hardware != "LG290P03" ||
		info.Firmware != "LG290P03AANR02A01S 2025/12/12" {
		t.Errorf("ReceiverInfo = %+v", info)
	}
	props := c.ConfigProps()
	if v, ok := props.GetBaudRate(); !ok || v != 460800 {
		t.Errorf("BaudRate = %v, %v", v, ok)
	}
	if v, ok := props.GetPort(); !ok || v != "UART1" {
		t.Errorf("Port = %q, %v", v, ok)
	}
	if v, ok := props.GetMinElevation(); !ok || v.Degrees() != 5.0 {
		t.Errorf("MinElevation = %v, %v", v, ok)
	}
	if v, ok := props.GetRTCMBaseID(); !ok || v != 290 {
		t.Errorf("RTCMBaseID = %v, %v", v, ok)
	}
	tp, ok := props.GetTimePulse()
	expectTP := gpsprot.TimePulse{
		Width:          100 * time.Millisecond,
		Period:         time.Second,
		AlignToGNSS:    true,
		OnlyWhenLocked: false, // as-found Mode 1 = always output
		PolarityRising: true,
	}
	if !ok || tp != expectTP {
		t.Errorf("TimePulse = %+v, %v, want %+v", tp, ok, expectTP)
	}
}

// TestConfiguratorDisabledPPS checks the truncated disabled-PPS
// readback (verbatim capture): only the zero width and the fixed
// alignment constant are knowable, the other pulse properties stay
// absent.
func TestConfiguratorDisabledPPS(t *testing.T) {
	responses := maps.Clone(fakeResponses)
	responses["PQTMCFGPPS2,R,1"] = []string{"PQTMCFGPPS2,OK,1,0"}
	c, errCount := runConfig(t, responses)
	if errCount != 0 {
		t.Errorf("director errors: %d", errCount)
	}
	props := c.ConfigProps()
	if v, ok := props.GetTimePulseWidth(); !ok || v != 0 {
		t.Errorf("TimePulseWidth = %v, %v, want 0, true", v, ok)
	}
	if v, ok := props.GetTimePulseAlignToGNSS(); !ok || !v {
		t.Errorf("TimePulseAlignToGNSS = %v, %v, want true, true", v, ok)
	}
	if _, ok := props.GetTimePulsePeriod(); ok {
		t.Error("TimePulsePeriod set despite disabled truncated readback")
	}
	if _, ok := props.GetTimePulseOnlyWhenLocked(); ok {
		t.Error("TimePulseOnlyWhenLocked set despite disabled truncated readback")
	}
}

// TestConfiguratorReducedCapability exercises the ERROR,3 drop paths:
// a receiver refusing PPS2 falls back to the legacy PPS query, and a
// refused ELETHD query leaves the tuple nil (absence), all without
// director errors.
func TestConfiguratorReducedCapability(t *testing.T) {
	responses := maps.Clone(fakeResponses)
	responses["PQTMCFGPPS2,R,1"] = []string{"PQTMCFGPPS2,ERROR,3"}
	responses["PQTMCFGELETHD,R"] = []string{"PQTMCFGELETHD,ERROR,3"}
	c, errCount := runConfig(t, responses)
	if errCount != 0 {
		t.Errorf("director errors: %d", errCount)
	}
	if c.found.pps2 != nil {
		t.Errorf("pps2 = %+v, want nil", c.found.pps2)
	}
	expect := &qtmmsg.CfgPPS{Index: 1, Enable: 1, Duration: 100, Mode: 1, Polarity: 1}
	if !reflect.DeepEqual(c.found.pps, expect) {
		t.Errorf("pps fallback:\ngot  %+v\nwant %+v", c.found.pps, expect)
	}
	if c.found.eleThd != nil {
		t.Errorf("eleThd = %+v, want nil", c.found.eleThd)
	}
}

// wSent filters the sent payloads down to the set commands.
func wSent(sent []string) []string {
	var w []string
	for _, p := range sent {
		if strings.Contains(p, ",W,") {
			w = append(w, p)
		}
	}
	return w
}

// TestConfiguratorNMEAMsgSets checks the complete-request semantics:
// every modeled member is set to its requested state, the named ones
// on and the rest off. The message table is not read back, so a set
// goes out even for members already in the requested state.
func TestConfiguratorNMEAMsgSets(t *testing.T) {
	responses := maps.Clone(fakeResponses)
	for _, p := range []string{"GGA,1", "GLL,0", "GSA,0", "GSV,0", "RMC,1", "VTG,0", "ZDA,0"} {
		responses["PQTMCFGMSGRATE,W,"+p] = []string{"PQTMCFGMSGRATE,OK"}
	}
	target := &gpsprot.ConfigTarget{}
	target.Opts.NMEAMsg.Set(gpsprot.NMEAMsgRMC | gpsprot.NMEAMsgGGA)
	_, errCount, sent := runConfigTarget(t, target, responses)
	if errCount != 0 {
		t.Errorf("director errors: %d", errCount)
	}
	expect := []string{
		"PQTMCFGMSGRATE,W,GGA,1",
		"PQTMCFGMSGRATE,W,GLL,0",
		"PQTMCFGMSGRATE,W,GSA,0",
		"PQTMCFGMSGRATE,W,GSV,0",
		"PQTMCFGMSGRATE,W,RMC,1",
		"PQTMCFGMSGRATE,W,VTG,0",
		"PQTMCFGMSGRATE,W,ZDA,0",
	}
	if got := wSent(sent); !reflect.DeepEqual(got, expect) {
		t.Errorf("sets sent:\ngot  %v\nwant %v", got, expect)
	}
}

// TestConfiguratorPVTMsgSets checks the PVT mapping for the serial-UTC
// timing set: PQTMPVT, EPE+DOP, EOE and SVINSTATUS are enabled with
// their versions and PVTMsgOff turns the other modeled PVT messages
// off; the mode-gated SVINSTATUS refusal (verbatim ERROR,1 capture)
// resolves as absence, not failure.
func TestConfiguratorPVTMsgSets(t *testing.T) {
	responses := maps.Clone(fakeResponses)
	for _, p := range []string{
		"PQTMDOP,1,1", "PQTMEOE,1,1", "PQTMEPE,1,2", "PQTMNAV,0,1", "PQTMODO,0,1",
		"PQTMPL,0,1", "PQTMPPPNAV,0,1", "PQTMPVT,1,1", "PQTMVEL,0,1",
	} {
		responses["PQTMCFGMSGRATE,W,"+p] = []string{"PQTMCFGMSGRATE,OK"}
	}
	responses["PQTMCFGMSGRATE,W,PQTMSVINSTATUS,1,1"] = []string{"PQTMCFGMSGRATE,ERROR,1"}
	target := &gpsprot.ConfigTarget{}
	target.Opts.PVTMsg = gpsprot.PVTMsgTimingSerialUTC | gpsprot.PVTMsgOff
	_, errCount, sent := runConfigTarget(t, target, responses)
	if errCount != 0 {
		t.Errorf("director errors: %d", errCount)
	}
	expect := []string{
		"PQTMCFGMSGRATE,W,PQTMDOP,1,1",
		"PQTMCFGMSGRATE,W,PQTMEOE,1,1",
		"PQTMCFGMSGRATE,W,PQTMEPE,1,2",
		"PQTMCFGMSGRATE,W,PQTMNAV,0,1",
		"PQTMCFGMSGRATE,W,PQTMODO,0,1",
		"PQTMCFGMSGRATE,W,PQTMPL,0,1",
		"PQTMCFGMSGRATE,W,PQTMPPPNAV,0,1",
		"PQTMCFGMSGRATE,W,PQTMPVT,1,1",
		"PQTMCFGMSGRATE,W,PQTMSVINSTATUS,1,1",
		"PQTMCFGMSGRATE,W,PQTMVEL,0,1",
	}
	if got := wSent(sent); !reflect.DeepEqual(got, expect) {
		t.Errorf("sets sent:\ngot  %v\nwant %v", got, expect)
	}
}

// TestConfiguratorMixedLevelsNoop checks that mixing message-level NMEA
// sentence control with a semantic axis (SatsMsg/PVTMsg) is
// contradictory on this native-NMEA receiver: the configurator does
// nothing for the NMEA/PVT/Sats output and leaves the NMEA protocol
// untouched (see plan/native-nmea.md).
func TestConfiguratorMixedLevelsNoop(t *testing.T) {
	responses := maps.Clone(fakeResponses)
	target := &gpsprot.ConfigTarget{}
	target.Opts.NMEAMsg.Set(gpsprot.NMEAMsgRMC | gpsprot.NMEAMsgGGA)
	target.Opts.SatsMsg.Set(gpsprot.SatsMsgAny)
	_, errCount, sent := runConfigTarget(t, target, responses)
	if errCount != 0 {
		t.Errorf("director errors: %d", errCount)
	}
	if got := wSent(sent); len(got) != 0 {
		t.Errorf("mixed request should send no message sets, got %v", got)
	}
}

// TestConfiguratorSemanticKeepsWire is the daemon's own request:
// NMEAMsg=None (semantic level, the native bridge) plus PVTMsg and
// SatsMsg. The native carriers (PQTM*, GSV) ride the NMEA protocol, so
// the protocol bit must stay on (no CFGPROT clear) while the standard
// sentences are disabled and the carriers enabled.
func TestConfiguratorSemanticKeepsWire(t *testing.T) {
	responses := maps.Clone(fakeResponses)
	for _, p := range []string{"GGA,0", "GLL,0", "GSA,0", "GSV,1", "RMC,0", "VTG,0", "ZDA,0",
		"PQTMDOP,1,1", "PQTMEOE,1,1", "PQTMEPE,1,2", "PQTMPVT,1,1"} {
		responses["PQTMCFGMSGRATE,W,"+p] = []string{"PQTMCFGMSGRATE,OK"}
	}
	responses["PQTMCFGMSGRATE,W,PQTMSVINSTATUS,1,1"] = []string{"PQTMCFGMSGRATE,ERROR,1"}
	target := &gpsprot.ConfigTarget{}
	target.Opts.NMEAMsg.Set(gpsprot.NMEAMsgNone)
	target.Opts.PVTMsg = gpsprot.PVTMsgTimingSerialUTC
	target.Opts.SatsMsg.Set(gpsprot.SatsMsgAny)
	_, errCount, sent := runConfigTarget(t, target, responses)
	if errCount != 0 {
		t.Errorf("director errors: %d", errCount)
	}
	w := wSent(sent)
	for _, p := range w {
		if strings.HasPrefix(p, "PQTMCFGPROT,W") {
			t.Errorf("NMEA protocol must not be toggled with carriers wanted: %s", p)
		}
	}
	for _, want := range []string{"PQTMCFGMSGRATE,W,GSV,1", "PQTMCFGMSGRATE,W,PQTMPVT,1,1",
		"PQTMCFGMSGRATE,W,GGA,0"} {
		if !slices.Contains(w, want) {
			t.Errorf("missing %q in %v", want, w)
		}
	}
}

// TestConfiguratorAllThreeOff checks that when NMEAMsg, PVTMsg and
// SatsMsg are all specified and all want nothing, the whole NMEA
// protocol is switched off in one CFGPROT write - the semantic level's
// only clearing case.
func TestConfiguratorAllThreeOff(t *testing.T) {
	responses := maps.Clone(fakeResponses)
	responses["PQTMCFGPROT,W,1,1,00000007,00000006"] = []string{"PQTMCFGPROT,OK"}
	target := &gpsprot.ConfigTarget{}
	target.Opts.NMEAMsg.Set(gpsprot.NMEAMsgNone)
	target.Opts.PVTMsg = gpsprot.PVTMsgOff
	target.Opts.SatsMsg.Set(gpsprot.SatsMsgNone)
	_, errCount, sent := runConfigTarget(t, target, responses)
	if errCount != 0 {
		t.Errorf("director errors: %d", errCount)
	}
	expect := []string{"PQTMCFGPROT,W,1,1,00000007,00000006"}
	if got := wSent(sent); !reflect.DeepEqual(got, expect) {
		t.Errorf("sets sent:\ngot  %v\nwant %v", got, expect)
	}
}

// TestConfiguratorSemanticNoClear checks that a bare semantic-off
// (PVTMsg=Off alone) never clears the NMEA protocol - it might silence
// standard NMEA the request was not told about. The PVT messages are
// disabled individually and the protocol bit is left as found.
func TestConfiguratorSemanticNoClear(t *testing.T) {
	responses := maps.Clone(fakeResponses)
	for _, p := range []string{"PQTMDOP,0,1", "PQTMEOE,0,1", "PQTMEPE,0,2", "PQTMNAV,0,1",
		"PQTMODO,0,1", "PQTMPL,0,1", "PQTMPPPNAV,0,1", "PQTMPVT,0,1", "PQTMSVINSTATUS,0,1",
		"PQTMVEL,0,1"} {
		responses["PQTMCFGMSGRATE,W,"+p] = []string{"PQTMCFGMSGRATE,OK"}
	}
	target := &gpsprot.ConfigTarget{}
	target.Opts.PVTMsg = gpsprot.PVTMsgOff
	_, errCount, sent := runConfigTarget(t, target, responses)
	if errCount != 0 {
		t.Errorf("director errors: %d", errCount)
	}
	w := wSent(sent)
	for _, p := range w {
		if strings.HasPrefix(p, "PQTMCFGPROT,W") {
			t.Errorf("semantic-off must not toggle the NMEA protocol: %s", p)
		}
	}
	if !slices.Contains(w, "PQTMCFGMSGRATE,W,PQTMPVT,0,1") {
		t.Errorf("PQTMPVT not disabled individually: %v", w)
	}
}

// TestConfiguratorNMEAAllOff checks that a complete request leaving no
// NMEA sentence on switches the whole NMEA output protocol off in one
// CFGPROT write (from the as-found 07) rather than disabling each
// sentence.
func TestConfiguratorNMEAAllOff(t *testing.T) {
	responses := maps.Clone(fakeResponses)
	responses["PQTMCFGPROT,W,1,1,00000007,00000006"] = []string{"PQTMCFGPROT,OK"}
	target := &gpsprot.ConfigTarget{}
	target.Opts.NMEAMsg.Set(gpsprot.NMEAMsgNone)
	_, errCount, sent := runConfigTarget(t, target, responses)
	if errCount != 0 {
		t.Errorf("director errors: %d", errCount)
	}
	expect := []string{"PQTMCFGPROT,W,1,1,00000007,00000006"}
	if got := wSent(sent); !reflect.DeepEqual(got, expect) {
		t.Errorf("sets sent:\ngot  %v\nwant %v", got, expect)
	}
}

// TestConfiguratorRTCMAllOff checks the same whole-protocol switch-off
// for RTCM: the RTCM3 output bit clears (07 -> 03) and no per-message
// disables go out.
func TestConfiguratorRTCMAllOff(t *testing.T) {
	responses := maps.Clone(fakeResponses)
	responses["PQTMCFGPROT,W,1,1,00000007,00000003"] = []string{"PQTMCFGPROT,OK"}
	target := &gpsprot.ConfigTarget{}
	target.Opts.RTCMMsg.Set(gpsprot.RTCMMsgNone)
	_, errCount, sent := runConfigTarget(t, target, responses)
	if errCount != 0 {
		t.Errorf("director errors: %d", errCount)
	}
	expect := []string{"PQTMCFGPROT,W,1,1,00000007,00000003"}
	if got := wSent(sent); !reflect.DeepEqual(got, expect) {
		t.Errorf("sets sent:\ngot  %v\nwant %v", got, expect)
	}
}

// TestConfiguratorNMEAReenable checks that when the NMEA output bit is
// found already cleared (a prior all-off), a request that wants NMEA
// back sets the bit on again so the per-message rates take effect.
func TestConfiguratorNMEAReenable(t *testing.T) {
	responses := maps.Clone(fakeResponses)
	responses["PQTMCFGPROT,R,1,1"] = []string{"PQTMCFGPROT,OK,1,1,00000007,00000006"}
	responses["PQTMCFGPROT,W,1,1,00000007,00000007"] = []string{"PQTMCFGPROT,OK"}
	for _, p := range []string{"GGA,0", "GLL,0", "GSA,0", "GSV,0", "RMC,1", "VTG,0", "ZDA,0"} {
		responses["PQTMCFGMSGRATE,W,"+p] = []string{"PQTMCFGMSGRATE,OK"}
	}
	target := &gpsprot.ConfigTarget{}
	target.Opts.NMEAMsg.Set(gpsprot.NMEAMsgRMC)
	_, errCount, sent := runConfigTarget(t, target, responses)
	if errCount != 0 {
		t.Errorf("director errors: %d", errCount)
	}
	w := wSent(sent)
	if len(w) == 0 || w[0] != "PQTMCFGPROT,W,1,1,00000007,00000007" {
		t.Errorf("NMEA bit not re-enabled first: %v", w)
	}
	if !slices.Contains(w, "PQTMCFGMSGRATE,W,RMC,1") {
		t.Errorf("RMC not enabled: %v", w)
	}
}

// TestConfiguratorNMEAOtherReenable checks that a request carrying the
// Other catch-all bit alongside a wanted sentence (as the web UI always
// sends) still re-enables the NMEA output protocol. The Other bit means
// "there may be sentences the model does not name", not "leave the
// protocol alone", so it must count toward enabling.
func TestConfiguratorNMEAOtherReenable(t *testing.T) {
	responses := maps.Clone(fakeResponses)
	responses["PQTMCFGPROT,R,1,1"] = []string{"PQTMCFGPROT,OK,1,1,00000007,00000006"}
	responses["PQTMCFGPROT,W,1,1,00000007,00000007"] = []string{"PQTMCFGPROT,OK"}
	for _, p := range []string{"GGA,0", "GLL,0", "GSA,0", "GSV,0", "RMC,1", "VTG,0", "ZDA,0"} {
		responses["PQTMCFGMSGRATE,W,"+p] = []string{"PQTMCFGMSGRATE,OK"}
	}
	target := &gpsprot.ConfigTarget{}
	target.Opts.NMEAMsg.Set(gpsprot.NMEAMsgRMC | gpsprot.NMEAMsgOther)
	_, errCount, sent := runConfigTarget(t, target, responses)
	if errCount != 0 {
		t.Errorf("director errors: %d", errCount)
	}
	w := wSent(sent)
	if len(w) == 0 || w[0] != "PQTMCFGPROT,W,1,1,00000007,00000007" {
		t.Errorf("NMEA bit not re-enabled first: %v", w)
	}
	if !slices.Contains(w, "PQTMCFGMSGRATE,W,RMC,1") {
		t.Errorf("RMC not enabled: %v", w)
	}
}

// TestConfiguratorProtOffNoReadback checks the fallback: with the
// CFGPROT readback refused, an all-off NMEA request cannot switch the
// protocol and disables each sentence individually instead.
func TestConfiguratorProtOffNoReadback(t *testing.T) {
	responses := maps.Clone(fakeResponses)
	responses["PQTMCFGPROT,R,1,1"] = []string{"PQTMCFGPROT,ERROR,3"}
	for _, p := range []string{"GGA,0", "GLL,0", "GSA,0", "GSV,0", "RMC,0", "VTG,0", "ZDA,0"} {
		responses["PQTMCFGMSGRATE,W,"+p] = []string{"PQTMCFGMSGRATE,OK"}
	}
	target := &gpsprot.ConfigTarget{}
	target.Opts.NMEAMsg.Set(gpsprot.NMEAMsgNone)
	_, errCount, sent := runConfigTarget(t, target, responses)
	if errCount != 0 {
		t.Errorf("director errors: %d", errCount)
	}
	expect := []string{
		"PQTMCFGMSGRATE,W,GGA,0",
		"PQTMCFGMSGRATE,W,GLL,0",
		"PQTMCFGMSGRATE,W,GSA,0",
		"PQTMCFGMSGRATE,W,GSV,0",
		"PQTMCFGMSGRATE,W,RMC,0",
		"PQTMCFGMSGRATE,W,VTG,0",
		"PQTMCFGMSGRATE,W,ZDA,0",
	}
	if got := wSent(sent); !reflect.DeepEqual(got, expect) {
		t.Errorf("sets sent:\ngot  %v\nwant %v", got, expect)
	}
}

// TestConfiguratorPropSets checks the property set paths: elevation
// and base ID writes with acknowledged assumed state, and the
// time-pulse read-modify-write choosing the legacy PPS form (which
// preserves the PPS2-only fields).
func TestConfiguratorPropSets(t *testing.T) {
	responses := maps.Clone(fakeResponses)
	responses["PQTMCFGELETHD,W,10.0"] = []string{"PQTMCFGELETHD,OK"}
	responses["PQTMCFGRSID,W,1024"] = []string{"PQTMCFGRSID,OK"}
	responses["PQTMCFGPPS,W,1,1,200,2,1,0"] = []string{"PQTMCFGPPS,OK"}
	target := &gpsprot.ConfigTarget{}
	target.Props.SetMinElevation(gpsprot.DegreesFromFloat(10))
	target.Props.SetRTCMBaseID(1024)
	target.Props.SetTimePulseWidth(200 * time.Millisecond)
	target.Props.SetTimePulseOnlyWhenLocked(true)
	c, errCount, sent := runConfigTarget(t, target, responses)
	if errCount != 0 {
		t.Errorf("director errors: %d", errCount)
	}
	expect := []string{
		"PQTMCFGELETHD,W,10.0",
		"PQTMCFGRSID,W,1024",
		"PQTMCFGPPS,W,1,1,200,2,1,0",
	}
	if got := wSent(sent); !reflect.DeepEqual(got, expect) {
		t.Errorf("sets sent:\ngot  %v\nwant %v", got, expect)
	}
	props := c.ConfigProps()
	if v, ok := props.GetMinElevation(); !ok || v.Degrees() != 10 {
		t.Errorf("MinElevation = %v, %v", v, ok)
	}
	if v, ok := props.GetRTCMBaseID(); !ok || v != 1024 {
		t.Errorf("RTCMBaseID = %v, %v", v, ok)
	}
	tp, ok := props.GetTimePulse()
	expectTP := gpsprot.TimePulse{
		Width:          200 * time.Millisecond,
		Period:         time.Second, // preserved PPS2 field
		AlignToGNSS:    true,
		OnlyWhenLocked: true,
		PolarityRising: true,
	}
	if !ok || tp != expectTP {
		t.Errorf("TimePulse = %+v, %v, want %+v", tp, ok, expectTP)
	}
}

// TestConfiguratorPropSetsNoChange checks that properties already at
// their requested values generate no sets at all.
func TestConfiguratorPropSetsNoChange(t *testing.T) {
	target := &gpsprot.ConfigTarget{}
	target.Props.SetMinElevation(gpsprot.DegreesFromFloat(5))
	target.Props.SetRTCMBaseID(290)
	target.Props.SetTimePulseWidth(100 * time.Millisecond)
	target.Props.SetTimePulsePolarityRising(true)
	_, errCount, sent := runConfigTarget(t, target, fakeResponses)
	if errCount != 0 {
		t.Errorf("director errors: %d", errCount)
	}
	if got := wSent(sent); got != nil {
		t.Errorf("unexpected sets: %v", got)
	}
}

// TestConfiguratorPPSDisable checks that a zero width disables the
// pulse with the truncated form the spec requires.
func TestConfiguratorPPSDisable(t *testing.T) {
	responses := maps.Clone(fakeResponses)
	responses["PQTMCFGPPS,W,1,0"] = []string{"PQTMCFGPPS,OK"}
	target := &gpsprot.ConfigTarget{}
	target.Props.SetTimePulseWidth(0)
	c, errCount, sent := runConfigTarget(t, target, responses)
	if errCount != 0 {
		t.Errorf("director errors: %d", errCount)
	}
	if got := wSent(sent); !reflect.DeepEqual(got, []string{"PQTMCFGPPS,W,1,0"}) {
		t.Errorf("sets sent: %v", got)
	}
	props := c.ConfigProps()
	if v, ok := props.GetTimePulseWidth(); !ok || v != 0 {
		t.Errorf("TimePulseWidth = %v, %v, want 0", v, ok)
	}
}

// TestConfiguratorPeriodRefused checks that a period other than the
// fixed 1 s is refused by the configurator itself: no pulse write goes
// out (writes use the legacy CfgPPS form only, which has no period).
func TestConfiguratorPeriodRefused(t *testing.T) {
	target := &gpsprot.ConfigTarget{}
	target.Props.SetTimePulsePeriod(2 * time.Second)
	_, errCount, sent := runConfigTarget(t, target, fakeResponses)
	if errCount == 0 {
		t.Error("no director error for a non-1s period")
	}
	if got := wSent(sent); len(got) != 0 {
		t.Errorf("sets sent: %v", got)
	}
}

// TestConfiguratorPeriodOneSecond checks that an explicit 1 s period
// is accepted and, matching the fixed period, generates no write.
func TestConfiguratorPeriodOneSecond(t *testing.T) {
	target := &gpsprot.ConfigTarget{}
	target.Props.SetTimePulsePeriod(time.Second)
	_, errCount, sent := runConfigTarget(t, target, fakeResponses)
	if errCount != 0 {
		t.Errorf("director errors: %d", errCount)
	}
	if got := wSent(sent); len(got) != 0 {
		t.Errorf("sets sent: %v", got)
	}
}

// lg290pDefaultSignals is the signal set denoted by the as-found
// masks (SIGNAL 07,03,0F,3F,0F,01 with every CNST gate on).
var lg290pDefaultSignals = gpsprot.SignalSetOf(
	gpsprot.SigGPSL1CA, gpsprot.SigGPSL2C, gpsprot.SigGPSL5,
	gpsprot.SigGLOL1, gpsprot.SigGLOL2,
	gpsprot.SigGALE1, gpsprot.SigGALE5a, gpsprot.SigGALE5b, gpsprot.SigGALE6,
	gpsprot.SigBDSB1I, gpsprot.SigBDSB2I, gpsprot.SigBDSB3I, gpsprot.SigBDSB1C,
	gpsprot.SigBDSB2a, gpsprot.SigBDSB2b,
	gpsprot.SigQZSSL1CA, gpsprot.SigQZSSL2C, gpsprot.SigQZSSL5, gpsprot.SigQZSSL6,
	gpsprot.SigNAVICL5)

// TestConfiguratorSignalsReadback checks the SignalsEnabled conversion
// from the as-found CFGSIGNAL masks and CFGCNST gates.
func TestConfiguratorSignalsReadback(t *testing.T) {
	c, errCount := runConfig(t, fakeResponses)
	if errCount != 0 {
		t.Errorf("director errors: %d", errCount)
	}
	if v, ok := c.ConfigProps().GetSignalsEnabled(); !ok || v != lg290pDefaultSignals {
		t.Errorf("SignalsEnabled = %v, %v, want %v", v, ok, lg290pDefaultSignals)
	}
}

// TestConfiguratorSignalsNoSaveReset checks the restart-only ruling:
// without an explicit save plus restart, a signal change is omitted
// and no stored-but-ineffective state is left behind - the skip shows
// in the unchanged readback, from which frontends derive warnings.
func TestConfiguratorSignalsNoSaveReset(t *testing.T) {
	target := &gpsprot.ConfigTarget{}
	target.Props.SetSignalsEnabled(lg290pDefaultSignals &^ gpsprot.SigSetGLO)
	c, errCount, sent := runConfigTarget(t, target, fakeResponses)
	if errCount != 0 {
		t.Errorf("director errors: %d", errCount)
	}
	if got := wSent(sent); len(got) != 0 {
		t.Errorf("sets sent without save+restart: %v", got)
	}
	if v, ok := c.ConfigProps().GetSignalsEnabled(); !ok || v != lg290pDefaultSignals {
		t.Errorf("readback = %v, %v, want as-found %v", v, ok, lg290pDefaultSignals)
	}
	target = &gpsprot.ConfigTarget{}
	target.Props.SetSignalsEnabled(lg290pDefaultSignals)
	_, errCount, sent = runConfigTarget(t, target, fakeResponses)
	if errCount != 0 {
		t.Errorf("director errors: %d", errCount)
	}
	if got := wSent(sent); len(got) != 0 {
		t.Errorf("sets sent for the current signal set: %v", got)
	}
}

// TestConfiguratorSignalsSaveReload checks disabling a constellation:
// only the CFGCNST gate changes (the mask is kept as found), the write
// precedes PQTMSAVEPAR, and the readback reports the new set.
func TestConfiguratorSignalsSaveReload(t *testing.T) {
	responses := maps.Clone(fakeResponses)
	responses["PQTMCFGCNST,W,1,0,1,1,1,1"] = []string{"PQTMCFGCNST,OK"}
	responses["PQTMSAVEPAR"] = []string{"PQTMSAVEPAR,OK"}
	target := &gpsprot.ConfigTarget{}
	target.Props.SetSignalsEnabled(lg290pDefaultSignals &^ gpsprot.SigSetGLO)
	target.Opts.Save = gpsprot.SaveMinimal
	target.Opts.Reset = gpsprot.ResetReload
	c, errCount, sent := runConfigTarget(t, target, responses)
	if errCount != 0 {
		t.Errorf("director errors: %d", errCount)
	}
	if got := wSent(sent); !reflect.DeepEqual(got, []string{"PQTMCFGCNST,W,1,0,1,1,1,1"}) {
		t.Errorf("sets sent: %v", got)
	}
	wi, si := -1, -1
	for i, p := range sent {
		switch p {
		case "PQTMCFGCNST,W,1,0,1,1,1,1":
			wi = i
		case "PQTMSAVEPAR":
			si = i
		}
	}
	if wi < 0 || si < 0 || wi >= si {
		t.Errorf("want CNST write before save; W at %d, SAVEPAR at %d (%v)", wi, si, sent)
	}
	if v, ok := c.ConfigProps().GetSignalsEnabled(); !ok || v != lg290pDefaultSignals&^gpsprot.SigSetGLO {
		t.Errorf("SignalsEnabled = %v, %v", v, ok)
	}
}

// TestConfiguratorModeNoSaveReset checks the restart-only ruling for
// the positioning mode: without an explicit save plus restart, a mode
// change is omitted - including a mobile request on a rover receiver
// with stored-but-inert SVIN state, whose effective mode already
// matches. The effective-mode readback shows the skip to frontends.
func TestConfiguratorModeNoSaveReset(t *testing.T) {
	target := &gpsprot.ConfigTarget{}
	target.Props.SetMode(gpsprot.Mode{Static: true})
	target.Opts.Survey = gpsprot.Survey{MinDur: 300 * time.Second, AccLimit: 2 * gpsprot.Meter}
	c, errCount, sent := runConfigTarget(t, target, fakeResponses)
	if errCount != 0 {
		t.Errorf("director errors: %d", errCount)
	}
	if got := wSent(sent); len(got) != 0 {
		t.Errorf("sets sent without save+restart: %v", got)
	}
	if m, ok := c.ConfigProps().GetMode(); !ok || m.Static {
		t.Errorf("readback mode = %+v, %v, want as-found mobile", m, ok)
	}
	responses := maps.Clone(fakeResponses)
	responses["PQTMCFGSVIN,R"] = []string{"PQTMCFGSVIN,OK,1,300,2.0,0.0000,0.0000,0.0000,0.0"}
	target = &gpsprot.ConfigTarget{}
	target.Props.SetMode(gpsprot.Mode{Static: false})
	_, errCount, sent = runConfigTarget(t, target, responses)
	if errCount != 0 {
		t.Errorf("director errors: %d", errCount)
	}
	if got := wSent(sent); len(got) != 0 {
		t.Errorf("sets sent without save+restart: %v", got)
	}
}

// TestConfiguratorModeSurvey checks the survey mode set: base receiver
// mode plus a survey-in SVIN tuple carrying the survey parameters,
// ordered before the save.
func TestConfiguratorModeSurvey(t *testing.T) {
	responses := maps.Clone(fakeResponses)
	responses["PQTMCFGRCVRMODE,W,2"] = []string{"PQTMCFGRCVRMODE,OK"}
	responses["PQTMCFGSVIN,W,1,300,2.3,0.0000,0.0000,0.0000"] = []string{"PQTMCFGSVIN,OK"}
	responses["PQTMSAVEPAR"] = []string{"PQTMSAVEPAR,OK"}
	target := &gpsprot.ConfigTarget{}
	target.Props.SetMode(gpsprot.Mode{Static: true})
	target.Opts.Survey = gpsprot.Survey{MinDur: 300 * time.Second,
		AccLimit: gpsprot.Length(2.3 * float64(gpsprot.Meter))}
	target.Opts.Save = gpsprot.SaveMinimal
	target.Opts.Reset = gpsprot.ResetReload
	_, errCount, sent := runConfigTarget(t, target, responses)
	if errCount != 0 {
		t.Errorf("director errors: %d", errCount)
	}
	want := []string{"PQTMCFGRCVRMODE,W,2", "PQTMCFGSVIN,W,1,300,2.3,0.0000,0.0000,0.0000"}
	if got := wSent(sent); !reflect.DeepEqual(got, want) {
		t.Errorf("sets sent: %v, want %v", got, want)
	}
	if n := len(sent); n < 2 || sent[n-2] != "PQTMSAVEPAR" || sent[n-1] != "PQTMSRR" {
		t.Errorf("tail of sent = %v, want [... PQTMSAVEPAR PQTMSRR]", sent[max(0, len(sent)-3):])
	}
}

// TestConfiguratorSetStatic checks the SetStatic option: on a rover
// receiver it acts as a static survey request under the save+reset
// gate, and an effectively static receiver is left as found, so an
// existing fixed position or survey is not disturbed.
func TestConfiguratorSetStatic(t *testing.T) {
	responses := maps.Clone(fakeResponses)
	responses["PQTMCFGRCVRMODE,W,2"] = []string{"PQTMCFGRCVRMODE,OK"}
	responses["PQTMCFGSVIN,W,1,300,2.3,0.0000,0.0000,0.0000"] = []string{"PQTMCFGSVIN,OK"}
	responses["PQTMSAVEPAR"] = []string{"PQTMSAVEPAR,OK"}
	target := &gpsprot.ConfigTarget{}
	target.Opts.SetStatic = true
	target.Opts.Survey = gpsprot.Survey{MinDur: 300 * time.Second,
		AccLimit: gpsprot.Length(2.3 * float64(gpsprot.Meter))}
	target.Opts.Save = gpsprot.SaveMinimal
	target.Opts.Reset = gpsprot.ResetReload
	_, errCount, sent := runConfigTarget(t, target, responses)
	if errCount != 0 {
		t.Errorf("director errors: %d", errCount)
	}
	want := []string{"PQTMCFGRCVRMODE,W,2", "PQTMCFGSVIN,W,1,300,2.3,0.0000,0.0000,0.0000"}
	if got := wSent(sent); !reflect.DeepEqual(got, want) {
		t.Errorf("sets sent: %v, want %v", got, want)
	}

	// Without save+reset the request is skipped like any mode change.
	target = &gpsprot.ConfigTarget{}
	target.Opts.SetStatic = true
	_, errCount, sent = runConfigTarget(t, target, fakeResponses)
	if errCount != 0 {
		t.Errorf("director errors: %d", errCount)
	}
	if got := wSent(sent); len(got) != 0 {
		t.Errorf("sets sent without save+restart: %v", got)
	}

	// An effectively static receiver (base + fixed SVIN) is left as
	// found even with save+reset.
	responses = maps.Clone(fakeResponses)
	responses["PQTMCFGRCVRMODE,R"] = []string{"PQTMCFGRCVRMODE,OK,2"}
	responses["PQTMCFGSVIN,R"] = []string{"PQTMCFGSVIN,OK,2,0,0.0,-1132881.1235,6092270.5679,1504542.9012,0.0"}
	responses["PQTMSAVEPAR"] = []string{"PQTMSAVEPAR,OK"}
	target = &gpsprot.ConfigTarget{}
	target.Opts.SetStatic = true
	target.Opts.Save = gpsprot.SaveMinimal
	target.Opts.Reset = gpsprot.ResetReload
	c, errCount, sent := runConfigTarget(t, target, responses)
	if errCount != 0 {
		t.Errorf("director errors: %d", errCount)
	}
	if got := wSent(sent); len(got) != 0 {
		t.Errorf("sets sent for an effectively static receiver: %v", got)
	}
	if m, ok := c.ConfigProps().GetMode(); !ok || !m.Static {
		t.Errorf("readback mode = %+v, %v, want static", m, ok)
	}
}

// TestConfiguratorModeFixedLLH checks the fixed-position set from LLH
// coordinates: converted to ECEF for the SVIN tuple, and the achieved
// mode reads back as a fixed ECEF position.
func TestConfiguratorModeFixedLLH(t *testing.T) {
	mode := gpsprot.Mode{Static: true, PosType: gpsprot.PosTypeLLH,
		FixedPosLLH: [2]gpsprot.Angle{gpsprot.DegreesFromFloat(13.7318284567),
			gpsprot.DegreesFromFloat(100.6447407891)},
		Height: gpsprot.Length(12.34567 * float64(gpsprot.Meter))}
	svin := fixedSvin(geopos.WGS84.LLHtoECEF(geopos.LLH{
		Lat: mode.FixedPosLLH[0].Degrees(), Lon: mode.FixedPosLLH[1].Degrees(),
		Height: mode.Height.Meters()}))
	payload, err := qtmmsg.EncodeWrite(&svin)
	if err != nil {
		t.Fatal(err)
	}
	responses := maps.Clone(fakeResponses)
	responses[payload] = []string{"PQTMCFGSVIN,OK"}
	responses["PQTMCFGRCVRMODE,W,2"] = []string{"PQTMCFGRCVRMODE,OK"}
	responses["PQTMSAVEPAR"] = []string{"PQTMSAVEPAR,OK"}
	target := &gpsprot.ConfigTarget{}
	target.Props.SetMode(mode)
	target.Opts.Save = gpsprot.SaveMinimal
	target.Opts.Reset = gpsprot.ResetReload
	c, errCount, sent := runConfigTarget(t, target, responses)
	if errCount != 0 {
		t.Errorf("director errors: %d", errCount)
	}
	if got := wSent(sent); !reflect.DeepEqual(got, []string{"PQTMCFGRCVRMODE,W,2", payload}) {
		t.Errorf("sets sent: %v, want RCVRMODE + %v", got, payload)
	}
	got, ok := c.ConfigProps().GetMode()
	if !ok || !got.Static || got.PosType != gpsprot.PosTypeECEF ||
		got.FixedPosECEF[0] != gpsprot.Meters(float64(svin.ECEFX)) {
		t.Errorf("Mode readback = %+v, %v", got, ok)
	}
}

// TestConfiguratorModeReadback checks the effective-mode conversion:
// a stored SVIN mode is inert in rover mode and reads back mobile,
// while base mode with a fixed position reads back static ECEF.
func TestConfiguratorModeReadback(t *testing.T) {
	responses := maps.Clone(fakeResponses)
	responses["PQTMCFGSVIN,R"] = []string{"PQTMCFGSVIN,OK,2,0,0.0,-1132881.1234,6092270.5678,1504542.9012,0.0"}
	c, errCount := runConfig(t, responses)
	if errCount != 0 {
		t.Errorf("director errors: %d", errCount)
	}
	if m, ok := c.ConfigProps().GetMode(); !ok || m.Static {
		t.Errorf("rover-mode readback = %+v, %v, want mobile (stored SVIN is inert)", m, ok)
	}
	responses["PQTMCFGRCVRMODE,R"] = []string{"PQTMCFGRCVRMODE,OK,2"}
	c, errCount = runConfig(t, responses)
	if errCount != 0 {
		t.Errorf("director errors: %d", errCount)
	}
	m, ok := c.ConfigProps().GetMode()
	if !ok || !m.Static || m.PosType != gpsprot.PosTypeECEF ||
		m.FixedPosECEF[0] != gpsprot.Meters(-1132881.1234) {
		t.Errorf("base-mode readback = %+v, %v, want static fixed ECEF", m, ok)
	}
}

// TestConfiguratorSignalsMasks checks per-signal masks: requested
// signals set their mask bits, unrequested constellations are gated
// off with their masks kept as found, and a requested signal the
// receiver has no mask bit for (GPS L1C) is dropped by intersection.
func TestConfiguratorSignalsMasks(t *testing.T) {
	responses := maps.Clone(fakeResponses)
	responses["PQTMCFGSIGNAL,W,01,03,01,3F,0F,01"] = []string{"PQTMCFGSIGNAL,OK"}
	responses["PQTMCFGCNST,W,1,0,1,0,0,0"] = []string{"PQTMCFGCNST,OK"}
	responses["PQTMSAVEPAR"] = []string{"PQTMSAVEPAR,OK"}
	target := &gpsprot.ConfigTarget{}
	target.Props.SetSignalsEnabled(gpsprot.SignalSetOf(
		gpsprot.SigGPSL1CA, gpsprot.SigGPSL1C, gpsprot.SigGALE1))
	target.Opts.Save = gpsprot.SaveMinimal
	target.Opts.Reset = gpsprot.ResetCold
	c, errCount, sent := runConfigTarget(t, target, responses)
	if errCount != 0 {
		t.Errorf("director errors: %d", errCount)
	}
	want := []string{"PQTMCFGSIGNAL,W,01,03,01,3F,0F,01", "PQTMCFGCNST,W,1,0,1,0,0,0"}
	if got := wSent(sent); !reflect.DeepEqual(got, want) {
		t.Errorf("sets sent: %v, want %v", got, want)
	}
	if n := len(sent); n < 2 || sent[n-2] != "PQTMSAVEPAR" || sent[n-1] != "PQTMCOLD" {
		t.Errorf("tail of sent = %v, want [... PQTMSAVEPAR PQTMCOLD]", sent[max(0, len(sent)-3):])
	}
	if v, ok := c.ConfigProps().GetSignalsEnabled(); !ok ||
		v != gpsprot.SignalSetOf(gpsprot.SigGPSL1CA, gpsprot.SigGALE1) {
		t.Errorf("SignalsEnabled = %v, %v", v, ok)
	}
}

// TestConfiguratorRTCMAuto checks RTCMMsgAuto: ARP and the MSM
// families enable (MSM sets carry the required offset field), and no
// CFGRTCM write is needed since the as-found MSM type is already 4.
func TestConfiguratorRTCMAuto(t *testing.T) {
	responses := maps.Clone(fakeResponses)
	responses["PQTMCFGMSGRATE,W,RTCM3-1005,1"] = []string{"PQTMCFGMSGRATE,OK"}
	for _, name := range rtcmMSMNames {
		responses["PQTMCFGMSGRATE,W,"+name+",1,0"] = []string{"PQTMCFGMSGRATE,OK"}
	}
	target := &gpsprot.ConfigTarget{}
	target.Opts.RTCMMsg.Set(gpsprot.RTCMMsgAuto)
	_, errCount, sent := runConfigTarget(t, target, responses)
	if errCount != 0 {
		t.Errorf("director errors: %d", errCount)
	}
	expect := []string{
		"PQTMCFGMSGRATE,W,RTCM3-1005,1",
		"PQTMCFGMSGRATE,W,RTCM3-107X,1,0",
		"PQTMCFGMSGRATE,W,RTCM3-108X,1,0",
		"PQTMCFGMSGRATE,W,RTCM3-109X,1,0",
		"PQTMCFGMSGRATE,W,RTCM3-111X,1,0",
		"PQTMCFGMSGRATE,W,RTCM3-112X,1,0",
		"PQTMCFGMSGRATE,W,RTCM3-113X,1,0",
	}
	if got := wSent(sent); !reflect.DeepEqual(got, expect) {
		t.Errorf("sets sent:\ngot  %v\nwant %v", got, expect)
	}
}

// TestConfiguratorRTCMMSM7 checks the MSM type gate: the type is
// stored-only, so with save+reset the CFGRTCM write goes out (rewriting
// the as-found tuple, before the save), and without save+reset it is
// suppressed - silently for a preference (Lax), with a warning for an
// explicit MSM7 request. Message enables are live and unaffected.
func TestConfiguratorRTCMMSM7(t *testing.T) {
	responses := maps.Clone(fakeResponses)
	responses["PQTMCFGMSGRATE,W,RTCM3-1005,1"] = []string{"PQTMCFGMSGRATE,OK"}
	for _, name := range rtcmMSMNames {
		responses["PQTMCFGMSGRATE,W,"+name+",1,0"] = []string{"PQTMCFGMSGRATE,OK"}
	}
	responses["PQTMCFGRTCM,W,7,0,-90.0,07,06,1,0"] = []string{"PQTMCFGRTCM,OK"}
	responses["PQTMSAVEPAR"] = []string{"PQTMSAVEPAR,OK"}

	target := &gpsprot.ConfigTarget{}
	target.Opts.RTCMMsg.Set(gpsprot.RTCMMsgAutoMSM7)
	target.Opts.Save = gpsprot.SaveMinimal
	target.Opts.Reset = gpsprot.ResetReload
	c, errCount, sent := runConfigTarget(t, target, responses)
	if errCount != 0 {
		t.Errorf("director errors: %d", errCount)
	}
	wi, si := -1, -1
	for i, p := range sent {
		switch p {
		case "PQTMCFGRTCM,W,7,0,-90.0,07,06,1,0":
			wi = i
		case "PQTMSAVEPAR":
			si = i
		}
	}
	if wi < 0 || si < 0 || wi >= si {
		t.Errorf("want CFGRTCM write before save; W at %d, SAVEPAR at %d (%v)", wi, si, wSent(sent))
	}
	if c.found.rtcm.MSMType != 7 {
		t.Errorf("assumed MSMType = %d, want 7", c.found.rtcm.MSMType)
	}

	// Without save+reset the CFGRTCM write is suppressed, for a
	// preference and an explicit MSM type alike.
	for _, flags := range []gpsprot.RTCMMsgFlags{gpsprot.RTCMMsgAutoMSM7, gpsprot.RTCMMsgMSM7 | gpsprot.RTCMMsgARP} {
		target = &gpsprot.ConfigTarget{}
		target.Opts.RTCMMsg.Set(flags)
		_, errCount, sent = runConfigTarget(t, target, responses)
		if errCount != 0 {
			t.Errorf("director errors: %d", errCount)
		}
		for _, p := range wSent(sent) {
			if strings.HasPrefix(p, "PQTMCFGRTCM,W") {
				t.Errorf("CFGRTCM write sent without save+reset: %s", p)
			}
		}
	}
}

// TestConfiguratorSpeedChange checks the baud-rate change when the
// host catches the write's OK at the new rate: the single CFGUART
// write succeeds on it, no repeat or extra query goes out, and the
// readback reports the new rate.
func TestConfiguratorSpeedChange(t *testing.T) {
	responses := maps.Clone(fakeResponses)
	responses["PQTMCFGUART,W,115200"] = []string{"PQTMCFGUART,OK"}
	target := &gpsprot.ConfigTarget{}
	target.Props.SetBaudRate(115200)
	c, errCount, sent := runConfigTarget(t, target, responses)
	if errCount != 0 {
		t.Errorf("director errors: %d", errCount)
	}
	writes, reads := 0, 0
	for _, p := range sent {
		switch p {
		case "PQTMCFGUART,W,115200":
			writes++
		case "PQTMCFGUART,R":
			reads++
		}
	}
	if writes != 1 || reads != 1 {
		t.Errorf("CFGUART writes %d, reads %d, want 1 write and 1 read (query only): %v", writes, reads, sent)
	}
	if v, ok := c.ConfigProps().GetBaudRate(); !ok || v != 115200 {
		t.Errorf("BaudRate readback = %v, %v, want 115200", v, ok)
	}
}

// TestConfiguratorSpeedChangeRepeat checks the lost-OK path (verbatim
// hardware: the port switches ~2 ms after the request, destroying the
// OK): the write stays awaiting, the same write is repeated after
// speedChangeRepeatDelay, and the OK elicited by the repeat confirms
// the original request.
func TestConfiguratorSpeedChangeRepeat(t *testing.T) {
	seq := map[string][][]string{
		"PQTMCFGUART,W,115200": {
			{},                  // the write's own OK is lost in the switch
			{"PQTMCFGUART,OK"}, // the repeat is received intact and answered
		},
	}
	target := &gpsprot.ConfigTarget{}
	target.Props.SetBaudRate(115200)
	c, errCount, sent := runConfigTargetSeq(t, target, fakeResponses, seq)
	if errCount != 0 {
		t.Errorf("director errors: %d", errCount)
	}
	writes := 0
	for _, p := range sent {
		if p == "PQTMCFGUART,W,115200" {
			writes++
		}
	}
	if writes != 2 {
		t.Errorf("CFGUART writes %d, want 2 (original and repeat): %v", writes, sent)
	}
	if v, ok := c.ConfigProps().GetBaudRate(); !ok || v != 115200 {
		t.Errorf("BaudRate readback = %v, %v, want 115200", v, ok)
	}
}

// TestSpeedChangeIncidentalConfirm checks that an incidental valid
// packet confirms a speed change only once speedChangeDelay has passed
// since the send.
func TestSpeedChangeIncidentalConfirm(t *testing.T) {
	confirmed := false
	r := &request{sentence: "PQTMCFGUART", speed: 115200, onOK: func() { confirmed = true }}
	t0 := time.Date(2026, 7, 7, 0, 0, 0, 0, time.UTC)
	r.SetSentTime(t0)
	r.MaybeSpeedChangeSucceeded(t0.Add(speedChangeDelay / 2))
	if confirmed || r.state != gpsprot.ConfigRequestAwaitingResponse {
		t.Errorf("confirmed by packet within speedChangeDelay: state %v", r.state)
	}
	r.MaybeSpeedChangeSucceeded(t0.Add(speedChangeDelay + time.Millisecond))
	if !confirmed || r.state != gpsprot.ConfigRequestSucceeded {
		t.Errorf("not confirmed: state %v, onOK called %v", r.state, confirmed)
	}
}

// TestConfiguratorSpeedNoChange checks that a target baud equal to the
// as-found rate generates no speed-change write and no confirm query.
func TestConfiguratorSpeedNoChange(t *testing.T) {
	target := &gpsprot.ConfigTarget{}
	target.Props.SetBaudRate(460800)
	_, errCount, sent := runConfigTarget(t, target, fakeResponses)
	if errCount != 0 {
		t.Errorf("director errors: %d", errCount)
	}
	uartReads := 0
	for _, p := range sent {
		if p == "PQTMCFGUART,R" {
			uartReads++
		}
		if strings.HasPrefix(p, "PQTMCFGUART,W") {
			t.Errorf("unexpected speed change: %s", p)
		}
	}
	if uartReads != 1 {
		t.Errorf("PQTMCFGUART,R sent %d times, want 1 (query only)", uartReads)
	}
}

// TestConfiguratorSpeedChangeSave checks the phase ordering: the speed
// change precedes PQTMSAVEPAR, so a save persists the new rate.
func TestConfiguratorSpeedChangeSave(t *testing.T) {
	responses := maps.Clone(fakeResponses)
	responses["PQTMCFGUART,W,115200"] = []string{"PQTMCFGUART,OK"}
	responses["PQTMSAVEPAR"] = []string{"PQTMSAVEPAR,OK"}
	target := &gpsprot.ConfigTarget{}
	target.Props.SetBaudRate(115200)
	target.Opts.Save = gpsprot.SaveAll
	_, errCount, sent := runConfigTarget(t, target, responses)
	if errCount != 0 {
		t.Errorf("director errors: %d", errCount)
	}
	wi, si := -1, -1
	for i, p := range sent {
		switch p {
		case "PQTMCFGUART,W,115200":
			wi = i
		case "PQTMSAVEPAR":
			si = i
		}
	}
	if wi < 0 || si < 0 || wi >= si {
		t.Errorf("want speed change before save; W at %d, SAVEPAR at %d (%v)", wi, si, sent)
	}
}

// TestConfiguratorSaveReload checks the NVM operations: SAVEPAR
// completes on its acknowledgement before the unacknowledged SRR
// goes out (phase-ordered, never pipelined - a reset would destroy
// the save's OK).
func TestConfiguratorSaveReload(t *testing.T) {
	responses := maps.Clone(fakeResponses)
	responses["PQTMSAVEPAR"] = []string{"PQTMSAVEPAR,OK"}
	target := &gpsprot.ConfigTarget{}
	target.Opts.Save = gpsprot.SaveAll
	target.Opts.Reset = gpsprot.ResetReload
	_, errCount, sent := runConfigTarget(t, target, responses)
	if errCount != 0 {
		t.Errorf("director errors: %d", errCount)
	}
	if n := len(sent); n < 2 || sent[n-2] != "PQTMSAVEPAR" || sent[n-1] != "PQTMSRR" {
		t.Errorf("tail of sent = %v, want [... PQTMSAVEPAR PQTMSRR]", sent[max(0, len(sent)-3):])
	}
}

// TestConfiguratorFactoryReset checks the factory sequence: the
// acknowledged PQTMRESTOREPAR completes before the PQTMSRR restart.
func TestConfiguratorFactoryReset(t *testing.T) {
	responses := maps.Clone(fakeResponses)
	responses["PQTMRESTOREPAR"] = []string{"PQTMRESTOREPAR,OK"}
	target := &gpsprot.ConfigTarget{}
	target.Opts.Reset = gpsprot.ResetFactory
	_, errCount, sent := runConfigTarget(t, target, responses)
	if errCount != 0 {
		t.Errorf("director errors: %d", errCount)
	}
	if n := len(sent); n < 2 || sent[n-2] != "PQTMRESTOREPAR" || sent[n-1] != "PQTMSRR" {
		t.Errorf("tail of sent = %v, want [... PQTMRESTOREPAR PQTMSRR]", sent[max(0, len(sent)-3):])
	}
}

// TestConfiguratorSilentReceiver checks that unanswered queries fail
// through the retry budget without deadlocking the director.
func TestConfiguratorSilentReceiver(t *testing.T) {
	c, errCount := runConfig(t, map[string][]string{})
	if errCount == 0 {
		t.Error("expected director errors for silent receiver")
	}
	count, complete := c.GetRequestCount()
	if !complete || count == 0 {
		t.Errorf("count=%d complete=%v", count, complete)
	}
}
