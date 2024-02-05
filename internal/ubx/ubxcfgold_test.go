package ubx

import (
	"fmt"
	"testing"
	"time"

	"github.com/jclark/satpulse/internal/gpsprot"
	ubxbin "github.com/jclark/satpulse/internal/ubx/bin"
)

func TestTp5(t *testing.T) {
	raw := &CfgOld{tp5: new(ubxbin.CfgTp5)}
	raw.tp5.Flags |= ubxbin.CfgTp5IsLength

	cm := &gpsprot.ConfigMap{}
	cm.SetPPS()

	raw.tp5 = raw.changeTp5(cm)

	ncm := gpsprot.ConfigMap{}
	raw.cookTp5(&ncm)
	bad := cm.Inconsistent(&ncm)
	if !bad.IsEmpty() {
		t.Errorf("tp5 change failed: %v", bad)
	}

	rep := raw.changeTp5(cm)

	if rep != nil {
		t.Errorf("repeated changeTp5 wasn't a no-op: %v", rep)
	}

	rep = raw.changeTp5(new(gpsprot.ConfigMap))
	if rep != nil {
		t.Errorf("changeTp5 with nothing wasn't a no-op: %v", rep)
	}
}

func TestChangeTp5GNSS(t *testing.T) {
	// Create a new RawConfig and Config
	raw := CfgOld{tp5: new(ubxbin.CfgTp5)}
	cm := gpsprot.ConfigMap{}

	// Call changeTp5GNSS with the empty RawConfig and Config
	gnss := raw.changeTp5GNSS(&cm)

	// Check that the result is gpsmsg.GPS
	if gnss != gpsprot.GPS {
		t.Errorf("expected gpsmsg.GPS, got %v", gnss)
	}
}

func TestNav5(t *testing.T) {
	raw := &CfgOld{nav5: new(ubxbin.CfgNav5)}

	cm := &gpsprot.ConfigMap{}
	cm.SetPPS()

	raw.nav5 = raw.changeNav5(cm)

	ncm := gpsprot.ConfigMap{}
	raw.cookNav5(&ncm)
	bad := cm.Inconsistent(&ncm)
	if !bad.IsEmpty() {
		t.Errorf("nav5 change failed: %v", bad)
	}

	rep := raw.changeNav5(cm)

	if rep != nil {
		t.Errorf("repeated changeNav5 wasn't a no-op: %v", rep)
	}

	rep = raw.changeNav5(new(gpsprot.ConfigMap))
	if rep != nil {
		t.Errorf("changeNav5 with nothing wasn't a no-op: %v", rep)
	}
}

func TestRate(t *testing.T) {
	raw := &CfgOld{rate: new(ubxbin.CfgRate)}
	raw.rate.NavRate = 1
	ver := new(Version)

	cm := &gpsprot.ConfigMap{}
	cm.SetPPS()

	raw.rate = raw.changeRate(cm, ver)

	ncm := gpsprot.ConfigMap{}
	raw.cookRate(&ncm, ver)
	bad := cm.Inconsistent(&ncm)
	if !bad.IsEmpty() {
		t.Errorf("rate change failed: %v", bad)
	}

	rep := raw.changeRate(cm, ver)

	if rep != nil {
		t.Errorf("repeated changeRate wasn't a no-op: %v", rep)
	}

	rep = raw.changeRate(new(gpsprot.ConfigMap), ver)
	if rep != nil {
		t.Errorf("changeRate with nothing wasn't a no-op: %v", rep)
	}
}

func TestConfiguratorSane(t *testing.T) {
	target := &gpsprot.ConfigMap{}
	target.SetPPS()
	testConfigurator(t, newLegacyReceiver(), target)
}

func TestConfiguratorGPS(t *testing.T) {
	target := &gpsprot.ConfigMap{}
	target.SetPPS()
	gpsprot.CfgPrimaryGNSS.Set(target, gpsprot.GPS)
	testConfigurator(t, newLegacyReceiver(), target)
}

func TestConfiguratorGalileo(t *testing.T) {
	target := &gpsprot.ConfigMap{}
	target.SetPPS()
	gpsprot.CfgPrimaryGNSS.Set(target, gpsprot.GAL)
	rcvr := newLegacyReceiver()
	rcvr.raw.gnss.Blocks[0].GNSSID = ubxbin.GAL
	testConfigurator(t, rcvr, target)
}

func testConfigurator(t *testing.T, rcvr gpsReceiver, target *gpsprot.ConfigMap) {
	c, naks, err := runConfiguration(rcvr, target, gpsprot.ConfigOptions{})
	if err != nil {
		t.Fatalf("unexpected error from runConfiguration: %v", err)
	}
	if len(naks) > 0 {
		t.Errorf("unexpected naks: %v", naks)
	}
	result := c.ConfigMap()

	bad := target.Inconsistent(result)
	if !bad.IsEmpty() {
		t.Errorf("final configuration is inconsistent: %v", bad)
	}
	missing := result.Missing(target)
	if !missing.IsEmpty() {
		t.Errorf("final configuration is missing: %v", missing)
	}
}

func TestConfiguratorRecover1(t *testing.T) {
	c := testConfiguratorRecover(t, ubxbin.CfgGNSSID)
	if c.raw.prt.OutProtoMask&ubxbin.CfgPrtProtoNMEA == 0 {
		t.Errorf("expected NMEA to be enabled, but it wasn't")
	}
}

func TestConfiguratorRecover2(t *testing.T) {
	c := testConfiguratorRecover(t, ubxbin.CfgRateID)
	// in this case we got far enough to enable the time message, so we don't need to disable NMEA
	if c.raw.prt.OutProtoMask&ubxbin.CfgPrtProtoNMEA != 0 {
		t.Errorf("expected NMEA not to be enabled, but it was")
	}
}

func testConfiguratorRecover(t *testing.T, nakMsgID ubxbin.MsgID) *Configurator {
	target := &gpsprot.ConfigMap{}
	target.SetPPS()
	gpsprot.CfgNMEAEnabled.Set(target, false)
	rcvr := newLegacyReceiver()
	rcvr.nakPollMsgID = nakMsgID
	c, naks, err := runConfiguration(rcvr, target, gpsprot.ConfigOptions{EnableTimeMsg: true})
	if err != nil {
		t.Errorf("unexpected error from runConfiguration: %v", err)
	}
	if len(naks) != 1 {
		t.Errorf("expected 1 nak, got %d", len(naks))
	}
	return c
}

type gpsReceiver interface {
	sendReceive(pkt []byte) [][]byte
	version() *Version
}

func runConfiguration(rcvr gpsReceiver, target *gpsprot.ConfigMap, options gpsprot.ConfigOptions) (*Configurator, []string, error) {
	prot := &Protocol{}
	prot.ver = rcvr.version()
	var naks []string

	c, err := prot.Configure(target, options)
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
		resps := rcvr.sendReceive(pkt)
		for _, resp := range resps {
			tm = tm.Add(time.Second / 10)
			err = prot.ProcessPacket(string(resp), tm)
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

type legacyReceiver struct {
	ver          *Version
	raw          *RawConfig
	nakPollMsgID ubxbin.MsgID
}

func newLegacyReceiver() *legacyReceiver {
	return &legacyReceiver{
		ver: new(Version),
		raw: newRawConfig(),
	}
}

func (r *legacyReceiver) version() *Version {
	return r.ver
}

func (r *legacyReceiver) sendReceive(pkt []byte) [][]byte {
	msgID := ubxbin.PacketMsgId(pkt)
	const pollMaxLen = 9
	var responses [][]byte

	if len(pkt) <= pollMaxLen {
		if msgID == r.nakPollMsgID {
			nak := ubxbin.AckNak{MsgID: msgID}
			resp, _ := ubxbin.Serialize(&nak)
			return [][]byte{resp}
		}
		var msg ubxbin.Msg
		switch msgID {
		case ubxbin.CfgPrtID:
			msg = r.raw.prt
		case ubxbin.CfgTp5ID:
			msg = r.raw.tp5
		case ubxbin.CfgNav5ID:
			msg = r.raw.nav5
		case ubxbin.CfgRateID:
			msg = r.raw.rate
		case ubxbin.CfgGNSSID:
			msg = r.raw.gnss
		}
		if msg != nil {
			if resp, err := ubxbin.Serialize(msg); err == nil {
				responses = append(responses, resp)
			}
		}

	}
	// XXX if it's not a poll we should update the receiver's state
	if msgID.Ackable() {
		ack := ubxbin.AckAck{MsgID: msgID}

		if resp, err := ubxbin.Serialize(&ack); err == nil {
			responses = append(responses, resp)
		}
	}
	return responses
}

func newRawConfig() *RawConfig {
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
		PulseLenRatioLock: 100,
		FreqPeriod:        1000,
		FreqPeriodLock:    1000,
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
	return &raw
}

func TestDivModRound(t *testing.T) {
	testCases := []struct {
		x, y, q int64
	}{
		{500, 100, 5},
		{550, 100, 6},
		{449, 100, 4},
		{-500, 100, -5},
		{-550, 100, -6},
		{-449, 100, -4},
		{17, 10, 2},
		{-17, 10, -2},
		{17, 1000, 0},
		{-17, 1000, 0},
		{1005, 1000, 1},
		{-1005, 1000, -1},
		{13, 10, 1},
		{15, 10, 2},
		{18, 10, 2},
		{-13, 10, -1},
		{-15, 10, -2},
		{-18, 10, -2},
	}

	for _, tc := range testCases {
		q, r := divModRound(tc.x, tc.y)
		if q != tc.q {
			t.Errorf("divModRound(%d, %d) = (%d, %d), want quotient %d",
				tc.x, tc.y, q, r, tc.q)
		}
		if tc.x != q*tc.y+r {
			t.Errorf("divModRound(%d, %d) = (%d, %d), does not satisfy x = quotient*y + remainder",
				tc.x, tc.y, q, r)
		}
	}
}
