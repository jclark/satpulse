package bin

import (
	"testing"

	"golang.org/x/exp/slices"
)

func TestCfgCfg(t *testing.T) {
	m := CfgCfg{
		CfgCfgFixed{
			ClearMask: CfgCfgIOPort,
			SaveMask:  CfgCfgRXMConf,
			LoadMask:  CfgCfgNavConf,
		},
		[]CfgCfgDeviceMask{CfgCfgDevFlash},
	}
	p2 := testMsgType1(t, m)
	if !EqualCfgCfg(&m, p2.(*CfgCfg)) {
		t.Fatalf("msg cfg-cfg not roundtripped %v => %v", &m, p2)
	}
	m = CfgCfg{
		CfgCfgFixed{
			ClearMask: CfgCfgIOPort,
			SaveMask:  CfgCfgRXMConf,
			LoadMask:  CfgCfgNavConf,
		},
		[]CfgCfgDeviceMask{},
	}
	p2 = testMsgType1(t, m)
	if !EqualCfgCfg(&m, p2.(*CfgCfg)) {
		t.Fatalf("msg cfg-cfg not roundtripped %v => %v", &m, p2)
	}
}

func EqualCfgCfg(p1, p2 *CfgCfg) bool {
	return p1.CfgCfgFixed == p2.CfgCfgFixed && slices.Equal(p1.DeviceMask, p2.DeviceMask)
}

func TestCfgValget(t *testing.T) {
	m := CfgValget{
		CfgValgetFixed{
			Version:  CfgValgetVersionResponse,
			Layer:    CfgValgetLayerRAM,
			Position: 0,
		},
		[]byte{1, 2, 3, 4, 5},
	}
	p2 := testMsgType1(t, m)
	if !EqualCfgValget(&m, p2.(*CfgValget)) {
		t.Fatalf("msg cfg-valget not roundtripped %v => %v", &m, p2)
	}
}

func EqualCfgValget(p1, p2 *CfgValget) bool {
	return p1.CfgValgetFixed == p2.CfgValgetFixed && slices.Equal(p1.CfgData, p2.CfgData)
}
