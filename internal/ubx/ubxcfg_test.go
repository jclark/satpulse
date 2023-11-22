package ubx

import (
	"testing"
	"time"

	"github.com/jclark/satpulse/internal/gpsmsg"
	ubxbin "github.com/jclark/satpulse/internal/ubx/bin"
)

func TestTp5(t *testing.T) {
	raw := &RawConfig{tp5: new(ubxbin.CfgTp5)}
	raw.tp5.Flags |= ubxbin.CfgTp5IsLength

	cm := &gpsmsg.ConfigMap{}
	cm.SetPPS()

	raw.tp5 = raw.changeTp5(cm)

	ncm := gpsmsg.ConfigMap{}
	raw.cookTp5(&ncm)
	bad := cm.Inconsistent(&ncm)
	if !bad.IsEmpty() {
		t.Errorf("tp5 change failed: %v", bad)
	}

	rep := raw.changeTp5(cm)

	if rep != nil {
		t.Errorf("repeated changeTp5 wasn't a no-op: %v", rep)
	}

	rep = raw.changeTp5(new(gpsmsg.ConfigMap))
	if rep != nil {
		t.Errorf("changeTp5 with nothing wasn't a no-op: %v", rep)
	}
}

func TestChangeTp5GNSS(t *testing.T) {
	// Create a new RawConfig and Config
	raw := RawConfig{tp5: new(ubxbin.CfgTp5)}
	cm := gpsmsg.ConfigMap{}

	// Call changeTp5GNSS with the empty RawConfig and Config
	gnss := raw.changeTp5GNSS(&cm)

	// Check that the result is gpsmsg.GPS
	if gnss != gpsmsg.GPS {
		t.Errorf("expected gpsmsg.GPS, got %v", gnss)
	}
}

func TestNav5(t *testing.T) {
	raw := &RawConfig{nav5: new(ubxbin.CfgNav5)}

	cm := &gpsmsg.ConfigMap{}
	cm.SetPPS()

	raw.nav5 = raw.changeNav5(cm)

	ncm := gpsmsg.ConfigMap{}
	raw.cookNav5(&ncm)
	bad := cm.Inconsistent(&ncm)
	if !bad.IsEmpty() {
		t.Errorf("nav5 change failed: %v", bad)
	}

	rep := raw.changeNav5(cm)

	if rep != nil {
		t.Errorf("repeated changeNav5 wasn't a no-op: %v", rep)
	}

	rep = raw.changeNav5(new(gpsmsg.ConfigMap))
	if rep != nil {
		t.Errorf("changeNav5 with nothing wasn't a no-op: %v", rep)
	}
}

func TestRate(t *testing.T) {
	raw := &RawConfig{rate: new(ubxbin.CfgRate)}
	raw.rate.NavRate = 1
	ver := new(Version)

	cm := &gpsmsg.ConfigMap{}
	cm.SetPPS()

	raw.rate = raw.changeRate(cm, ver)

	ncm := gpsmsg.ConfigMap{}
	raw.cookRate(&ncm, ver)
	bad := cm.Inconsistent(&ncm)
	if !bad.IsEmpty() {
		t.Errorf("rate change failed: %v", bad)
	}

	rep := raw.changeRate(cm, ver)

	if rep != nil {
		t.Errorf("repeated changeRate wasn't a no-op: %v", rep)
	}

	rep = raw.changeRate(new(gpsmsg.ConfigMap), ver)
	if rep != nil {
		t.Errorf("changeRate with nothing wasn't a no-op: %v", rep)
	}
}

func TestConfiguratorSane(t *testing.T) {
	testConfigurator(t, func(raw *RawConfig, target *gpsmsg.ConfigMap, ver *Version) {
		target.SetPPS()
	})

}

func TestConfiguratorGPS(t *testing.T) {
	testConfigurator(t, func(raw *RawConfig, target *gpsmsg.ConfigMap, ver *Version) {
		target.SetPPS()
		gpsmsg.CfgPrimaryGNSS.Set(target, gpsmsg.GPS)
	})
}

func TestConfiguratorGalileo(t *testing.T) {
	testConfigurator(t, func(raw *RawConfig, target *gpsmsg.ConfigMap, ver *Version) {
		target.SetPPS()
		raw.gnss.Blocks[0].GNSSID = ubxbin.GAL
		gpsmsg.CfgPrimaryGNSS.Set(target, gpsmsg.GAL)
	})
}

func testConfigurator(t *testing.T, setup func(*RawConfig, *gpsmsg.ConfigMap, *Version)) *Configurator {
	raw := newRawConfig()
	target := &gpsmsg.ConfigMap{}
	ver := &Version{}
	setup(raw, target, ver)

	prot := &Protocol{}
	prot.ver = ver

	c, err := prot.Configure(target, gpsmsg.ConfigOptions{})
	if err != nil {
		t.Fatalf("unexpected error from Configure: %v", err)
	}
	tm := time.Now()
	for {
		tm = tm.Add(time.Second / 10)
		req, err := c.NextRequest()
		if err != nil {
			t.Errorf("unexpected error from NextRequest: %v", err)
			break
		}
		if req == nil {
			break
		}
		const pollMaxLen = 9
		pkt := req.Packet()
		msgID := ubxbin.PacketMsgId(pkt)
		if len(pkt) <= pollMaxLen {
			var msg ubxbin.Msg
			switch msgID {
			case ubxbin.CfgPrtID:
				msg = raw.prt
			case ubxbin.CfgTp5ID:
				msg = raw.tp5
			case ubxbin.CfgNav5ID:
				msg = raw.nav5
			case ubxbin.CfgRateID:
				msg = raw.rate
			case ubxbin.CfgGNSSID:
				msg = raw.gnss
			}
			if msg != nil {
				resp, err := ubxbin.Serialize(msg)
				if err != nil {
					t.Errorf("unexpected serialization error: %v", err)
				} else {
					err = prot.ProcessPacket(string(resp), tm)
					if err != nil {
						t.Errorf("unexpected error processing response packet: %v", err)
					}
				}
			}
		}
		if req.Ackable() {
			req.Done()
		}
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

	uc := c.(*Configurator)
	return uc
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

func TestSplitLength(t *testing.T) {
	const mm10 = gpsmsg.Micrometer * 100

	testCases := []struct {
		length       gpsmsg.Length
		expectedCm   int32
		expectedMm10 int8
	}{
		{105 * mm10, 1, 5},
		{250 * mm10, 3, -50},
		{-105 * mm10, -1, -5},
		{-250 * mm10, -3, 50},
		{10475 * gpsmsg.Micrometer, 1, 5},
	}

	for _, tc := range testCases {
		cm, mm10, err := splitLength(tc.length)
		if err != nil {
			t.Errorf("splitLength returned error: %v", err)
		} else if mm10 < -99 || mm10 > 99 {
			t.Errorf("splitLength(%v) = (%v, %v), want mm10 in [-99, 99]", tc.length, cm, mm10)
		} else if cm != tc.expectedCm || mm10 != tc.expectedMm10 {
			t.Errorf("splitLength(%v) = (%v, %v), want (%v, %v)", tc.length, cm, mm10, tc.expectedCm, tc.expectedMm10)
		}
	}
}
