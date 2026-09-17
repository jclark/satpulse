package ubxsim

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/jclark/satpulse/gps/app/gpsio"
	"github.com/jclark/satpulse/gps/internal/nmea"
	"github.com/jclark/satpulse/gps/internal/rtcm"
	"github.com/jclark/satpulse/gps/internal/ubx"
	"github.com/jclark/satpulse/gps/lib/rtcmbin"
	"github.com/jclark/satpulse/gps/lib/ubxbin"
	ucv "github.com/jclark/satpulse/gps/lib/ubxcfgval"
)

func writeLog(t *testing.T, path string, entries []gpsio.PacketLogEntry) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	for i := range entries {
		if err := enc.Encode(&entries[i]); err != nil {
			t.Fatalf("encode: %v", err)
		}
	}
}

func valgetResponse(t *testing.T, layer ubxbin.CfgValgetLayer, items []ucv.Item) []byte {
	t.Helper()
	data, err := ucv.MarshalItems(items)
	if err != nil {
		t.Fatalf("marshal items: %v", err)
	}
	pkt, err := ubxbin.Serialize(&ubxbin.CfgValget{
		CfgValgetFixed: ubxbin.CfgValgetFixed{Version: ubxbin.CfgValgetVersionResponse, Layer: layer},
		CfgData:        data,
	})
	if err != nil {
		t.Fatalf("serialize: %v", err)
	}
	return pkt
}

// testMsm7 returns a serialized GPS MSM7 (1077) packet.
func testMsm7(t *testing.T) string {
	t.Helper()
	m7 := &rtcmbin.MSMHiRes{
		MSMHeader: rtcmbin.MSMHeader{
			MsgHdrStationID: rtcmbin.MsgHdrStationID{
				MsgHdr:    rtcmbin.MsgHdr{MsgNum: 1077},
				StationID: 1234,
			},
			SatMask: 1 << 63,
			SigMask: 1 << 31,
		},
		CellMask: 1 << 0,
		Sat: rtcmbin.MSMSatData{
			RangeInt:  rtcmbin.Uint8Slice{100},
			ExtInfo:   rtcmbin.Uint8Slice{1},
			RangeMod:  []uint16{500},
			PhaseRate: []int16{10},
		},
		Sig: rtcmbin.MSMHiResSigData{
			Pseudorange: []int32{1000},
			PhaseRange:  []int32{2000},
			LockTime:    []uint16{100},
			HalfCycle:   []bool{true},
			CNR:         []uint16{160},
			PhaseRate:   []int16{100},
		},
	}
	pkt, err := rtcmbin.SerializeMsg(m7)
	if err != nil {
		t.Fatalf("serialize MSM7: %v", err)
	}
	return pkt
}

func TestLoadPersonality(t *testing.T) {
	dir := t.TempDir()
	personalityPath := filepath.Join(dir, "personality.ubx")
	replayPath := filepath.Join(dir, "replay.jsonl")
	navSat := ucv.KUbxNavSat.KeyU(ucv.UART1).Key()
	gga := ucv.KNmeaIdGga.KeyU(ucv.UART1).Key()
	t0 := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	monVer := testMonVer()
	monGnss, _ := ubxbin.PackMsg(ubxbin.MonGnssID, make([]byte, 8))
	ack, err := ubxbin.Serialize(&ubxbin.AckAck{MsgID: ubxbin.CfgValgetID})
	if err != nil {
		t.Fatalf("serialize ACK: %v", err)
	}
	var ubxData []byte
	ubxData = append(ubxData, monVer...)
	ubxData = append(ubxData, monGnss...)
	// A RAM-layer response and the capture's ACK traffic are ignored
	// by the personality loader.
	ubxData = append(ubxData, valgetResponse(t, ubxbin.CfgValgetLayerRAM,
		[]ucv.Item{{Key: navSat, Value: 9}})...)
	ubxData = append(ubxData, valgetResponse(t, ubxbin.CfgValgetLayerDefault,
		[]ucv.Item{{Key: navSat, Value: 0}})...)
	ubxData = append(ubxData, ack...)
	ubxData = append(ubxData, valgetResponse(t, ubxbin.CfgValgetLayerDefault,
		[]ucv.Item{{Key: gga, Value: 1}})...)
	if err := os.WriteFile(personalityPath, ubxData, 0o644); err != nil {
		t.Fatalf("write personality: %v", err)
	}
	navSatPkt := testNavSat()
	ggaPkt := testGga()
	monHw, _ := ubxbin.PackMsg(ubxbin.MakeMsgID(0x0a, 0x09), make([]byte, 60))
	msm7 := testMsm7(t)
	msm4, err := rtcmbin.MSM7ConvertPacket(msm7, 4)
	if err != nil {
		t.Fatalf("MSM7ConvertPacket: %v", err)
	}
	writeLog(t, replayPath, []gpsio.PacketLogEntry{
		{T: gpsio.TimeMicro(t0), Tag: ubx.Tag, Bin: navSatPkt},
		{T: gpsio.TimeMicro(t0.Add(50 * time.Millisecond)), Tag: nmea.Tag, Ascii: string(ggaPkt)},
		// No MSGOUT key for MON-HW: skipped.
		{T: gpsio.TimeMicro(t0.Add(60 * time.Millisecond)), Tag: ubx.Tag, Bin: monHw},
		{T: gpsio.TimeMicro(t0.Add(70 * time.Millisecond)), Tag: rtcm.Tag, Bin: gpsio.HexString(msm7)},
		{T: gpsio.TimeMicro(t0.Add(time.Second)), Tag: ubx.Tag, Bin: navSatPkt},
	})
	p, err := LoadPersonality(personalityPath)
	if err != nil {
		t.Fatalf("LoadPersonality: %v", err)
	}
	if err := p.LoadReplay(replayPath); err != nil {
		t.Fatalf("LoadReplay: %v", err)
	}
	expect := &Personality{
		MonVer:   monVer,
		MonGnss:  monGnss,
		Defaults: ucv.Map{navSat: 0, gga: 1},
		Epochs: [][]Pkt{
			{
				{KeyM: ucv.KUbxNavSat, Tag: ubx.Tag, Data: navSatPkt},
				{KeyM: ucv.KNmeaIdGga, Tag: nmea.Tag, Data: ggaPkt},
				// The recorded MSM7 also yields a derived MSM4 entry.
				{KeyM: ucv.KRtcm3xType1077, Tag: rtcm.Tag, Data: []byte(msm7)},
				{KeyM: ucv.KRtcm3xType1074, Tag: rtcm.Tag, Data: []byte(msm4)},
			},
			{{KeyM: ucv.KUbxNavSat, Tag: ubx.Tag, Data: navSatPkt}},
		},
		Skipped: 1,
	}
	if !reflect.DeepEqual(p, expect) {
		t.Errorf("got  %+v\nwant %+v", p, expect)
	}
}

// TestLoadPersonalityF9P loads the checked-in ZED-F9P personality file
// (HPG 1.51, captured with the capture kit).
func TestLoadPersonalityF9P(t *testing.T) {
	p, err := LoadPersonality("testdata/f9p/f9p-personality.ubx")
	if err != nil {
		t.Fatalf("LoadPersonality: %v", err)
	}
	if len(p.Defaults) != 1499 {
		t.Errorf("got %d Default-layer items, want 1499", len(p.Defaults))
	}
	if !bytes.Contains(p.MonVer, []byte("MOD=ZED-F9P")) {
		t.Errorf("MON-VER does not identify a ZED-F9P")
	}
	if p.MonGnss == nil {
		t.Errorf("no MON-GNSS")
	}
}
