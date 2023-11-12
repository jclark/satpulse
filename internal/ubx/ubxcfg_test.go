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

	cfg := &gpsmsg.Config{}
	cfg.SetSane()

	raw.tp5 = raw.changeTp5(cfg)

	ncfg := gpsmsg.Config{}
	raw.cookTp5(&ncfg)
	bad := cfg.Inconsistent(&ncfg)
	if !bad.IsEmpty() {
		t.Errorf("tp5 change failed: %v", bad)
	}

	rep := raw.changeTp5(cfg)

	if rep != nil {
		t.Errorf("repeated changeTp5 wasn't a no-op: %v", rep)
	}

	rep = raw.changeTp5(new(gpsmsg.Config))
	if rep != nil {
		t.Errorf("changeTp5 with nothing wasn't a no-op: %v", rep)
	}
}

func TestChangeTp5GNSS(t *testing.T) {
	// Create a new RawConfig and Config
	raw := RawConfig{tp5: new(ubxbin.CfgTp5)}
	cfg := gpsmsg.Config{}

	// Call changeTp5GNSS with the empty RawConfig and Config
	gnss := raw.changeTp5GNSS(&cfg)

	// Check that the result is gpsmsg.GPS
	if gnss != gpsmsg.GPS {
		t.Errorf("expected gpsmsg.GPS, got %v", gnss)
	}
}

func TestNav5(t *testing.T) {
	raw := &RawConfig{nav5: new(ubxbin.CfgNav5)}

	cfg := &gpsmsg.Config{}
	cfg.SetSane()

	raw.nav5 = raw.changeNav5(cfg)

	ncfg := gpsmsg.Config{}
	raw.cookNav5(&ncfg)
	bad := cfg.Inconsistent(&ncfg)
	if !bad.IsEmpty() {
		t.Errorf("nav5 change failed: %v", bad)
	}

	rep := raw.changeNav5(cfg)

	if rep != nil {
		t.Errorf("repeated changeNav5 wasn't a no-op: %v", rep)
	}

	rep = raw.changeNav5(new(gpsmsg.Config))
	if rep != nil {
		t.Errorf("changeNav5 with nothing wasn't a no-op: %v", rep)
	}
}

func TestRate(t *testing.T) {
	raw := &RawConfig{rate: new(ubxbin.CfgRate)}
	raw.rate.NavRate = 1
	ver := new(Version)

	cfg := &gpsmsg.Config{}
	cfg.SetSane()

	raw.rate = raw.changeRate(cfg, ver)

	ncfg := gpsmsg.Config{}
	raw.cookRate(&ncfg, ver)
	bad := cfg.Inconsistent(&ncfg)
	if !bad.IsEmpty() {
		t.Errorf("rate change failed: %v", bad)
	}

	rep := raw.changeRate(cfg, ver)

	if rep != nil {
		t.Errorf("repeated changeRate wasn't a no-op: %v", rep)
	}

	rep = raw.changeRate(new(gpsmsg.Config), ver)
	if rep != nil {
		t.Errorf("changeRate with nothing wasn't a no-op: %v", rep)
	}
}

func TestConfiguratorSane(t *testing.T) {
	testConfigurator(t, func(raw *RawConfig, target *gpsmsg.Config, ver *Version) {
		target.SetSane()
	})

}

func TestConfiguratorGPS(t *testing.T) {
	testConfigurator(t, func(raw *RawConfig, target *gpsmsg.Config, ver *Version) {
		target.SetSane()
		gpsmsg.CfgPrimaryGNSS.Set(target, gpsmsg.GPS)
	})
}

func TestConfiguratorGalileo(t *testing.T) {
	testConfigurator(t, func(raw *RawConfig, target *gpsmsg.Config, ver *Version) {
		target.SetSane()
		raw.gnss.Blocks[0].GNSSID = ubxbin.Galileo
		gpsmsg.CfgPrimaryGNSS.Set(target, gpsmsg.Galileo)
	})
}

func testConfigurator(t *testing.T, setup func(*RawConfig, *gpsmsg.Config, *Version)) *Configurator {
	raw := newRawConfig()
	target := &gpsmsg.Config{}
	ver := &Version{}
	setup(raw, target, ver)

	prot := &Protocol{}
	prot.ver = ver

	c := prot.Configure(target)
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
					err = prot.ProcessPacket(string(resp), tm, nil, nil)
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

	result := c.Config()

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
