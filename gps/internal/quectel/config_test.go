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

// runConfig drives a ConfigProtocol through probe and Configure and
// runs the real ConfigDirector against the fake receiver's response
// map, returning the Configurator and the director's error count.
func runConfig(t *testing.T, responses map[string][]string) (*Configurator, int) {
	t.Helper()
	cp := NewConfigProtocol()
	now := time.Date(2026, 7, 7, 0, 0, 0, 0, time.UTC)
	feed(t, cp, vernoPayload, now)
	if !cp.ProbeOK() {
		t.Fatal("ProbeOK false after PQTMVERNO response")
	}
	cfgtor, err := cp.Configure(&gpsprot.ConfigTarget{})
	if err != nil {
		t.Fatalf("Configure: %v", err)
	}
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
	return cfgtor.(*Configurator), dir.ErrorCount
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
