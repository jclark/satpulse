package ubxbin

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

func TestCfgInf(t *testing.T) {
	notice := CfgInfError | CfgInfWarning | CfgInfNotice
	m := CfgInf{[]CfgInfBlock{
		{ProtocolID: CfgInfProtoUBX, InfMsgMask: [NPort]CfgInfMask{CfgInfError}},
		{ProtocolID: CfgInfProtoNMEA, InfMsgMask: [NPort]CfgInfMask{notice, notice, 0, notice, notice, 0}},
	}}
	p2 := testMsgType1(t, m)
	if !slices.Equal(m.Blocks, p2.(*CfgInf).Blocks) {
		t.Fatalf("msg cfg-inf not roundtripped %v => %v", &m, p2)
	}
	// NMEA defaults as reported by a LEA-M8T running TIM 1.10.
	packet, err := PackMsg(CfgInfID, []byte{1, 0, 0, 0, 0x07, 0x07, 0x00, 0x07, 0x07, 0x00})
	if err != nil {
		t.Fatalf("PackMsg: %v", err)
	}
	msg, err := ParseMsg(string(packet))
	if err != nil {
		t.Fatalf("failed to parse CFG-INF: %v", err)
	}
	want := []CfgInfBlock{{ProtocolID: CfgInfProtoNMEA, InfMsgMask: [NPort]CfgInfMask{notice, notice, 0, notice, notice, 0}}}
	if got := msg.(*CfgInf).Blocks; !slices.Equal(got, want) {
		t.Errorf("CFG-INF blocks: got %v, want %v", got, want)
	}
}
