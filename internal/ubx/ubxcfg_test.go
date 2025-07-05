package ubx

import (
	"fmt"
	"testing"
	"time"

	"github.com/jclark/satpulse/internal/gpsprot"
	ubxbin "github.com/jclark/satpulse/internal/ubx/bin"
)


func TestConfigurationGet_Legacy(t *testing.T) {
	testConfigurationGet(t, newLegacyReceiver())
}

var m10Version = Version{
	HW:   "000S0000",
	SW:   "ROM SPG 5.10 (c00d69)",
	Prot: &ProtVer{Major: 34, Minor: 10},
	FW:   &FWVer{ProductCategory: "SPG", Major: 5, Minor: 10},
	GNSS: gpsprot.MajorGNSSSet,
}

var m8tVersion = Version{
	Prot: &ProtVer{Major: 22, Minor: 00},
	Mod:  "LEA-M8T-0",
	FW:   &FWVer{ProductCategory: "TIM", Major: 8, Minor: 01},
}

func TestConfigurationEmpty_New(t *testing.T) {
	testConfigurationEmpty(t, &m10Version)
}

func TestConfigurationEmpty_Legacy(t *testing.T) {
	testConfigurationEmpty(t, &m8tVersion)
}

func testConfigurationEmpty(t *testing.T, ver *Version) {
	target := gpsprot.NewConfigTarget()
	rcvr := newGpsReceiver(ver)
	rcvr.raw.tmode2 = &ubxbin.CfgTmode2{
		TimeMode: ubxbin.CfgTmode2SurveyIn,
	}
	_, _, err := runConfiguration(rcvr, target)
	if err != nil {
		t.Errorf("unexpected error from runConfiguration: %v", err)
	}
	if rcvr.nSent != 0 {
		t.Errorf("expected no messages to be sent, got %d", rcvr.nSent)
	}
}

func TestConfigurationGet_New(t *testing.T) {
	testConfigurationGet(t, &gpsReceiver{
		version: &m10Version,
		raw:     newDefaultRawConfig(),
	})
}

func newLegacyReceiver() *gpsReceiver {
	return newGpsReceiver(new(Version))
}

func newGpsReceiver(ver *Version) *gpsReceiver {
	return &gpsReceiver{
		version: ver,
		raw:     newDefaultRawConfig(),
	}
}

func testConfigurationGet(t *testing.T, rcvr *gpsReceiver) {
	target := gpsprot.NewConfigTarget()
	target.Get |= gpsprot.PropIDTimePulseWidth
	c, naks, err := runConfiguration(rcvr, target)
	if err != nil {
		t.Fatalf("unexpected error from runConfiguration: %v", err)
	}
	if len(naks) != 0 {
		t.Errorf("expected no naks, got %v", naks)
	}
	if w, ok := c.ConfigProps().GetTimePulseWidth(); !ok || w != time.Second/10 {
		t.Errorf("expected pulse width to be 0.1s, got %v", w)
	}
	if rcvr.nSent != 1 {
		t.Errorf("expected 1 message to be sent, got %d", rcvr.nSent)
	}
}

type gpsReceiver struct {
	version      *Version
	raw          *RawConfig
	nakPollMsgID ubxbin.MsgID
	abortMsgID   ubxbin.MsgID  // simulate abort (timeout/corruption) when sending this message
	nSent        int
}

func runConfiguration(rcvr *gpsReceiver, target *gpsprot.ConfigTarget) (*Configurator, []string, error) {
	pp := NewPacketProcessor()
	px := pp.CreatePacketExchanger()
	upx := px.(*PacketExchanger)
	upx.ver = rcvr.version
	var naks []string

	c, err := px.Configure(target)
	if err != nil {
		return nil, nil, fmt.Errorf("unexpected error from Configure: %w", err)
	}
	tm := time.Now()
	for {
		tm = tm.Add(time.Second / 10)
		req, err := c.NextRequest()
		if err != nil {
			return nil, nil, fmt.Errorf("unexpected error from NextRequest: %w", err)
		}
		if req == nil {
			break
		}
		sendTm := tm
		pkt := req.Packet()
		msgID := ubxbin.PacketMsgId(pkt)
		
		// Simulate abort scenario (timeout/corruption)
		if msgID == rcvr.abortMsgID {
			// This simulates what happens in gpscfg when there's a non-NACK error
			c.Abort()
			// Continue to allow the configurator to generate recovery requests
			continue
		}
		
		resps := rcvr.sendReceive(pkt)
		for _, resp := range resps {
			tm = tm.Add(time.Second / 10)
			_, err = pp.ProcessPacket(string(resp), tm)
			if err != nil {
				return nil, nil, fmt.Errorf("unexpected error processing response packet: %w", err)
			}
		}
		ack := c.FindAck(pkt, sendTm)
		if ack == nil {
			return nil, nil, fmt.Errorf("no ack found for request %s", req.ID())
		} else if ack.OK {
			req.Done()
		} else {
			naks = append(naks, req.ID())
		}
	}
	uc := c.(*Configurator)
	return uc, naks, nil
}

func (r *gpsReceiver) sendReceive(pkt []byte) [][]byte {
	r.nSent++
	msgID := ubxbin.PacketMsgId(pkt)
	const pollMaxLen = 9
	var respPkts [][]byte
	var respMsg ubxbin.Msg
	nak := false

	if msgID == ubxbin.CfgValgetID {
		respMsg = r.valgetResp(pkt)
		if respMsg == nil {
			nak = true
		}
	} else if len(pkt) <= pollMaxLen {
		switch msgID {
		case ubxbin.CfgPrtID:
			respMsg = r.raw.prt
		case ubxbin.CfgTp5ID:
			respMsg = r.raw.tp5
		case ubxbin.CfgNav5ID:
			respMsg = r.raw.nav5
		case ubxbin.CfgRateID:
			respMsg = r.raw.rate
		case ubxbin.CfgGNSSID:
			respMsg = r.raw.gnss
		case ubxbin.CfgTmode2ID:
			respMsg = r.raw.tmode2
		case ubxbin.CfgTmode3ID:
			respMsg = r.raw.tmode3
		}
	}
	if msgID == r.nakPollMsgID {
		nak = true
		respMsg = nil
	}
	if respMsg != nil {
		if respPkt, err := ubxbin.Serialize(respMsg); err == nil {
			respPkts = append(respPkts, respPkt)
		}
	}
	// XXX if it's not a poll we should update the receiver's state
	if msgID.Ackable() {
		var ackMsg ubxbin.Msg
		if nak {
			ackMsg = &ubxbin.AckNak{MsgID: msgID}
		} else {
			ackMsg = &ubxbin.AckAck{MsgID: msgID}
		}
		if resp, err := ubxbin.Serialize(ackMsg); err == nil {
			respPkts = append(respPkts, resp)
		}
	}
	return respPkts
}

func newDefaultRawConfig() *RawConfig {
	raw := RawConfig{}
	raw.prt = &ubxbin.CfgPrt{
		PortID:       ubxbin.PortUART1,
		InProtoMask:  ubxbin.CfgPrtProtoUBX | ubxbin.CfgPrtProtoNMEA,
		OutProtoMask: ubxbin.CfgPrtProtoUBX | ubxbin.CfgPrtProtoNMEA,
	}
	raw.tp5 = &ubxbin.CfgTp5{
		Flags:             ubxbin.CfgTp5IsLength | ubxbin.CfgTp5Active | ubxbin.CfgTp5LockGpsFreq | ubxbin.CfgTp5Polarity | ubxbin.CfgTp5AlignToTow | ubxbin.CfgTp5LockedOtherSet,
		Version:           1,
		PulseLenRatio:     0,
		PulseLenRatioLock: 100 * 1000,
		FreqPeriod:        1000 * 1000,
		FreqPeriodLock:    1000 * 1000,
		AntCableDelay:     50,
	}
	raw.nav5 = &ubxbin.CfgNav5{
		DynModel:    ubxbin.CfgNav5DynPortable,
		UtcStandard: ubxbin.CfgNav5UtcAuto,
	}
	raw.rate = &ubxbin.CfgRate{
		MeasRate: 1000,
		NavRate:  1,
		TimeRef:  ubxbin.CfgRateUTC,
	}
	raw.gnss = &ubxbin.CfgGNSS{
		CfgGNSSFixed: ubxbin.CfgGNSSFixed{NumConfigBlocks: 1},
		Blocks:       []ubxbin.CfgGNSSBlock{{GNSSID: ubxbin.GPS}},
	}
	cfgValsInit(raw.valsPtr())
	return &raw
}

