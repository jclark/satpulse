package quectel

import (
	"maps"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/internal/nmea"
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
	"PQTMLSTMSG": {
		"PQTMLSTMSG,OK,1,1,GGA,1",
		"PQTMLSTMSG,OK,1,1,GLL,1",
		"PQTMLSTMSG,OK,1,1,GSA,1",
		"PQTMLSTMSG,OK,1,1,GSV,1",
		"PQTMLSTMSG,OK,1,1,RMC,1",
		"PQTMLSTMSG,OK,1,1,VTG,1",
		"PQTMLSTMSG,OK,1,1,PQTMTXT,1,1",
		"PQTMLSTMSG,OK,End",
	},
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
			for _, resp := range responses[payload] {
				now = now.Add(time.Millisecond)
				dir.AdvanceTimeTo(now)
				feed(t, cp, resp, now)
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
		lstOK:    true,
		msgRates: c.found.msgRates, // compared separately below
	}
	if !reflect.DeepEqual(c.found, expect) {
		t.Errorf("as-found state:\ngot  %+v\nwant %+v", c.found, expect)
	}
	if len(c.found.msgRates) != 7 {
		t.Errorf("msgRates len = %d, want 7", len(c.found.msgRates))
	}
	if got := c.found.msgRates["GGA"]; got == nil || got.Rate != 1 {
		t.Errorf("msgRates[GGA] = %+v", got)
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
// modeled members go to the requested state and unmodeled standard
// NMEA messages in the as-found table turn off (no NMEAMsgOther).
// Members already in the requested state generate no set.
func TestConfiguratorNMEAMsgSets(t *testing.T) {
	responses := maps.Clone(fakeResponses)
	for _, name := range []string{"GLL", "GSA", "GSV", "VTG"} {
		responses["PQTMCFGMSGRATE,W,"+name+",0"] = []string{"PQTMCFGMSGRATE,OK"}
	}
	target := &gpsprot.ConfigTarget{}
	target.Opts.NMEAMsg.Set(gpsprot.NMEAMsgRMC | gpsprot.NMEAMsgGGA)
	c, errCount, sent := runConfigTarget(t, target, responses)
	if errCount != 0 {
		t.Errorf("director errors: %d", errCount)
	}
	expect := []string{
		"PQTMCFGMSGRATE,W,GLL,0",
		"PQTMCFGMSGRATE,W,GSA,0",
		"PQTMCFGMSGRATE,W,GSV,0",
		"PQTMCFGMSGRATE,W,VTG,0",
	}
	if got := wSent(sent); !reflect.DeepEqual(got, expect) {
		t.Errorf("sets sent:\ngot  %v\nwant %v", got, expect)
	}
	for _, name := range []string{"GLL", "GSA", "GSV", "VTG"} {
		if c.msgEnabled(name) {
			t.Errorf("%s still enabled in assumed state", name)
		}
	}
	if !c.msgEnabled("RMC") || !c.msgEnabled("GGA") {
		t.Error("RMC/GGA lost from assumed state")
	}
}

// TestConfiguratorPVTMsgSets checks the PVT mapping for the serial-UTC
// timing set: PQTMPVT, EPE+DOP, EOE and SVINSTATUS are enabled with
// their versions; the mode-gated SVINSTATUS refusal (verbatim ERROR,1
// capture) resolves as absence, not failure.
func TestConfiguratorPVTMsgSets(t *testing.T) {
	responses := maps.Clone(fakeResponses)
	for _, p := range []string{"PQTMPVT,1,1", "PQTMEPE,1,2", "PQTMDOP,1,1", "PQTMEOE,1,1"} {
		responses["PQTMCFGMSGRATE,W,"+p] = []string{"PQTMCFGMSGRATE,OK"}
	}
	responses["PQTMCFGMSGRATE,W,PQTMSVINSTATUS,1,1"] = []string{"PQTMCFGMSGRATE,ERROR,1"}
	target := &gpsprot.ConfigTarget{}
	target.Opts.PVTMsg = gpsprot.PVTMsgTimingSerialUTC | gpsprot.PVTMsgOff
	c, errCount, sent := runConfigTarget(t, target, responses)
	if errCount != 0 {
		t.Errorf("director errors: %d", errCount)
	}
	expect := []string{
		"PQTMCFGMSGRATE,W,PQTMDOP,1,1",
		"PQTMCFGMSGRATE,W,PQTMEOE,1,1",
		"PQTMCFGMSGRATE,W,PQTMEPE,1,2",
		"PQTMCFGMSGRATE,W,PQTMPVT,1,1",
		"PQTMCFGMSGRATE,W,PQTMSVINSTATUS,1,1",
	}
	if got := wSent(sent); !reflect.DeepEqual(got, expect) {
		t.Errorf("sets sent:\ngot  %v\nwant %v", got, expect)
	}
	for _, name := range []string{"PQTMPVT", "PQTMEPE", "PQTMDOP", "PQTMEOE"} {
		if !c.msgEnabled(name) {
			t.Errorf("%s not in assumed state", name)
		}
	}
	if c.msgEnabled("PQTMSVINSTATUS") {
		t.Error("refused SVINSTATUS recorded as enabled")
	}
}

// TestConfiguratorSatsMsgUnion checks that SatsMsg keeps GSV on even
// when the NMEA flags would turn it off, and that no redundant set is
// sent when the union matches the as-found state.
func TestConfiguratorSatsMsgUnion(t *testing.T) {
	responses := maps.Clone(fakeResponses)
	for _, name := range []string{"GLL", "GSA", "VTG"} {
		responses["PQTMCFGMSGRATE,W,"+name+",0"] = []string{"PQTMCFGMSGRATE,OK"}
	}
	target := &gpsprot.ConfigTarget{}
	target.Opts.NMEAMsg.Set(gpsprot.NMEAMsgRMC | gpsprot.NMEAMsgGGA)
	target.Opts.SatsMsg.Set(gpsprot.SatsMsgAny)
	c, errCount, sent := runConfigTarget(t, target, responses)
	if errCount != 0 {
		t.Errorf("director errors: %d", errCount)
	}
	for _, p := range wSent(sent) {
		if strings.Contains(p, "GSV") {
			t.Errorf("unexpected GSV set: %s", p)
		}
	}
	if !c.msgEnabled("GSV") {
		t.Error("GSV lost despite SatsMsg request")
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

// TestConfiguratorPPS2Period checks that a non-default period selects
// the PPS2 form with the full extended tuple.
func TestConfiguratorPPS2Period(t *testing.T) {
	responses := maps.Clone(fakeResponses)
	responses["PQTMCFGPPS2,W,1,1,100,1,1,0,2000,0,1,0,0,0"] = []string{"PQTMCFGPPS2,OK"}
	target := &gpsprot.ConfigTarget{}
	target.Props.SetTimePulsePeriod(2 * time.Second)
	c, errCount, sent := runConfigTarget(t, target, responses)
	if errCount != 0 {
		t.Errorf("director errors: %d", errCount)
	}
	if got := wSent(sent); !reflect.DeepEqual(got, []string{"PQTMCFGPPS2,W,1,1,100,1,1,0,2000,0,1,0,0,0"}) {
		t.Errorf("sets sent: %v", got)
	}
	if v, ok := c.ConfigProps().GetTimePulsePeriod(); !ok || v != 2*time.Second {
		t.Errorf("TimePulsePeriod = %v, %v", v, ok)
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
	c, errCount, sent := runConfigTarget(t, target, responses)
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
	if !c.msgEnabled("RTCM3-1005") || !c.msgEnabled("RTCM3-112X") {
		t.Error("RTCM messages missing from assumed state")
	}
}

// TestConfiguratorRTCMMSM7 checks that requesting MSM7 rewrites
// CFGRTCM with the new MSM type over the as-found tuple.
func TestConfiguratorRTCMMSM7(t *testing.T) {
	responses := maps.Clone(fakeResponses)
	responses["PQTMCFGMSGRATE,W,RTCM3-1005,1"] = []string{"PQTMCFGMSGRATE,OK"}
	for _, name := range rtcmMSMNames {
		responses["PQTMCFGMSGRATE,W,"+name+",1,0"] = []string{"PQTMCFGMSGRATE,OK"}
	}
	responses["PQTMCFGRTCM,W,7,0,-90.0,07,06,1,0"] = []string{"PQTMCFGRTCM,OK"}
	target := &gpsprot.ConfigTarget{}
	target.Opts.RTCMMsg.Set(gpsprot.RTCMMsgAutoMSM7)
	c, errCount, sent := runConfigTarget(t, target, responses)
	if errCount != 0 {
		t.Errorf("director errors: %d", errCount)
	}
	found := false
	for _, p := range wSent(sent) {
		if p == "PQTMCFGRTCM,W,7,0,-90.0,07,06,1,0" {
			found = true
		}
	}
	if !found {
		t.Errorf("CFGRTCM MSM7 write not sent: %v", wSent(sent))
	}
	if c.found.rtcm.MSMType != 7 {
		t.Errorf("assumed MSMType = %d, want 7", c.found.rtcm.MSMType)
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
