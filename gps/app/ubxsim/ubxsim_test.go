package ubxsim

import (
	"bytes"
	"fmt"
	"io"
	"reflect"
	"strings"
	"testing"
	"testing/synctest"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/internal/nmea"
	"github.com/jclark/satpulse/gps/internal/ubx"
	"github.com/jclark/satpulse/gps/lib/nmeamsg"
	"github.com/jclark/satpulse/gps/lib/rtcmbin"
	"github.com/jclark/satpulse/gps/lib/spartnbin"
	"github.com/jclark/satpulse/gps/lib/ubxbin"
	ucv "github.com/jclark/satpulse/gps/lib/ubxcfgval"
	"github.com/jclark/satpulse/gps/scan"
)

func testMonVer() []byte {
	payload := make([]byte, 40+30)
	copy(payload, "EXT CORE 1.00 (deadbe)")
	copy(payload[30:], "00190000")
	copy(payload[40:], "PROTVER=50.11")
	pkt, err := ubxbin.PackMsg(ubxbin.MonVerID, payload)
	if err != nil {
		panic(err)
	}
	return pkt
}

func testNavSat() []byte {
	pkt, err := ubxbin.PackMsg(ubxbin.NavSatID, make([]byte, 8))
	if err != nil {
		panic(err)
	}
	return pkt
}

func testGga() []byte {
	body := "GPGGA,000000.00,,,,,0,00,99.99,,,,,,"
	return fmt.Appendf(nil, "$%s*%02X\r\n", body, nmeamsg.Checksum([]byte(body)))
}

func testPersonality() *Personality {
	dflt := testDefaults()
	dflt[ucv.KUart1Baudrate.Key()] = 38400
	epochs := make([][]Pkt, 60)
	for i := range epochs {
		epochs[i] = []Pkt{
			{KeyM: ucv.KUbxNavSat, Data: testNavSat()},
			{KeyM: ucv.KNmeaIdGga, Data: testGga()},
		}
	}
	return &Personality{MonVer: testMonVer(), Defaults: dflt, Epochs: epochs}
}

type rwPair struct {
	io.Reader
	io.Writer
}

// simConn runs a Sim over in-process pipes and returns the test-side
// write end, a channel of packets the simulator emits, and a shutdown
// function.
func simConn(t *testing.T, p *Personality) (io.Writer, <-chan scan.Packet) {
	simRead, testWrite := io.Pipe()
	testRead, simWrite := io.Pipe()
	sim := New(p, Options{})
	done := make(chan error, 1)
	go func() {
		done <- sim.Run(t.Context(), rwPair{simRead, simWrite})
	}()
	ch := make(chan scan.Packet)
	go func() {
		sc := scan.New(testRead, 4096, []gpsprot.PacketFormat{ubx.PacketFormat, nmea.PacketFormat})
		for {
			pkt, err := sc.Scan()
			if err != nil {
				close(ch)
				return
			}
			if pkt.Format != nil {
				ch <- pkt
			}
		}
	}()
	t.Cleanup(func() {
		testWrite.Close()
		if err := <-done; err != nil {
			t.Errorf("sim.Run: %v", err)
		}
		testRead.Close()
		for range ch {
		}
	})
	return testWrite, ch
}

// await reads packets until match accepts one, failing the test if the
// channel closes first.
func await(t *testing.T, ch <-chan scan.Packet, what string, match func(scan.Packet) bool) scan.Packet {
	t.Helper()
	for pkt := range ch {
		if match(pkt) {
			return pkt
		}
	}
	t.Fatalf("simulator output ended awaiting %s", what)
	return scan.Packet{}
}

func isUbx(mid ubxbin.MsgID) func(scan.Packet) bool {
	return func(p scan.Packet) bool {
		return p.Format == ubx.PacketFormat && ubxbin.PacketMsgId(p.Data) == mid
	}
}

func isNmea(typ string) func(scan.Packet) bool {
	return func(p scan.Packet) bool {
		return p.Format == nmea.PacketFormat && strings.HasPrefix(p.Data[3:], typ)
	}
}

// ackResult reads packets until an ACK-ACK or ACK-NAK for mid arrives
// and reports whether it was an ACK-ACK.
func ackResult(t *testing.T, ch <-chan scan.Packet, mid ubxbin.MsgID) bool {
	t.Helper()
	var ack bool
	await(t, ch, "ACK/NAK", func(p scan.Packet) bool {
		pmid := ubxbin.PacketMsgId(p.Data)
		if p.Format != ubx.PacketFormat || (pmid != ubxbin.AckAckID && pmid != ubxbin.AckNakID) {
			return false
		}
		if ubxbin.AckMsgID(p.Data) != mid {
			return false
		}
		ack = pmid == ubxbin.AckAckID
		return true
	})
	return ack
}

func sendMsg(t *testing.T, w io.Writer, m ubxbin.Msg) {
	t.Helper()
	pkt, err := ubxbin.Serialize(m)
	if err != nil {
		t.Fatalf("serialize: %v", err)
	}
	if _, err := w.Write(pkt); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func TestSim(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		p := testPersonality()
		navSat := ucv.KUbxNavSat.KeyU(ucv.UART1).Key()
		w, ch := simConn(t, p)

		// The factory-default personality emits its default NMEA set.
		await(t, ch, "GGA", isNmea("GGA"))

		// MON-VER poll is answered with the recorded packet verbatim.
		if _, err := w.Write(ubxbin.Poll(ubxbin.MonVerID)); err != nil {
			t.Fatalf("write: %v", err)
		}
		mv := await(t, ch, "MON-VER", isUbx(ubxbin.MonVerID))
		if !bytes.Equal([]byte(mv.Data), p.MonVer) {
			t.Errorf("MON-VER not replayed verbatim")
		}

		// A VALGET poll gets its response and then an ACK, interleaved
		// with nav traffic.
		sendMsg(t, w, valgetPoll(ubxbin.CfgValgetLayerRAM, 0, navSat))
		resp := await(t, ch, "CFG-VALGET response", isUbx(ubxbin.CfgValgetID))
		m, err := ubxbin.ParseMsg(resp.Data)
		if err != nil {
			t.Fatalf("parse response: %v", err)
		}
		items, err := ucv.UnmarshalItems(m.(*ubxbin.CfgValget).CfgData)
		if err != nil || len(items) != 1 || items[0] != (ucv.Item{Key: navSat, Value: 0}) {
			t.Errorf("bad VALGET response items %v err %v", items, err)
		}
		if !ackResult(t, ch, ubxbin.CfgValgetID) {
			t.Errorf("VALGET poll NAKed")
		}

		// Enabling a message via VALSET makes it appear in the stream.
		sendMsg(t, w, valsetMsg(ubxbin.CfgValsetLayerRAM, ucv.Item{Key: navSat, Value: 1}))
		if !ackResult(t, ch, ubxbin.CfgValsetID) {
			t.Fatalf("VALSET NAKed")
		}
		await(t, ch, "NAV-SAT", isUbx(ubxbin.NavSatID))

		// A VALSET with a key this personality does not have is NAKed
		// and nothing is applied.
		sendMsg(t, w, valsetMsg(ubxbin.CfgValsetLayerRAM,
			ucv.Item{Key: ucv.KUbxTimSvin.KeyU(ucv.UART1).Key(), Value: 1}))
		if ackResult(t, ch, ubxbin.CfgValsetID) {
			t.Errorf("VALSET of unknown key ACKed")
		}
	})
}

// testSpartn returns a serialized SPARTN frame with the given
// encryption flag.
func testSpartn(t *testing.T, eaf bool) []byte {
	t.Helper()
	f := &spartnbin.Frame{
		FrameStart:    spartnbin.FrameStart{Type: 1, EAF: eaf},
		FrameMetadata: spartnbin.FrameMetadata{Subtype: 3},
		Payload:       []byte{1, 2, 3, 4},
	}
	pkt, err := spartnbin.Pack(f)
	if err != nil {
		t.Fatalf("pack SPARTN: %v", err)
	}
	return pkt
}

func TestRxmCor(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		rtcm1230, err := rtcmbin.PackMsg([]byte{0x4c, 0xe1, 0x23}) // TYPE1230, station ID 291
		if err != nil {
			t.Fatalf("pack RTCM: %v", err)
		}
		common := ubxbin.RxmCorErrStatusErrorFree | ubxbin.RxmCorMsgUsedUsed |
			ubxbin.RxmCorMsgTypeValid | ubxbin.RxmCorMsgInputHandle
		tests := []struct {
			name   string
			in     []byte
			expect *ubxbin.RxmCor
		}{
			{
				name: "RTCM with station ID",
				in:   rtcm1230,
				expect: &ubxbin.RxmCor{
					Version: 1,
					StatusInfo: common | ubxbin.RxmCorProtocolRTCM3 |
						ubxbin.RxmCorMsgEncryptedNotEncrypted | 291<<9,
					MsgType: 1230,
				},
			},
			{
				name: "SPARTN",
				in:   testSpartn(t, false),
				expect: &ubxbin.RxmCor{
					Version: 1,
					StatusInfo: common | ubxbin.RxmCorProtocolSPARTN | ubxbin.RxmCorMsgSubTypeValid |
						ubxbin.RxmCorMsgEncryptedNotEncrypted | 0xffff<<9,
					MsgType:    1,
					MsgSubType: 3,
				},
			},
			{
				name: "SPARTN encrypted",
				in:   testSpartn(t, true),
				expect: &ubxbin.RxmCor{
					Version: 1,
					StatusInfo: common | ubxbin.RxmCorProtocolSPARTN | ubxbin.RxmCorMsgSubTypeValid |
						ubxbin.RxmCorMsgEncryptedEncrypted | 0xffff<<9,
					MsgType:    1,
					MsgSubType: 3,
				},
			},
		}
		p := &Personality{MonVer: testMonVer(), Defaults: testDefaults()}
		w, ch := simConn(t, p)

		// With RXM-COR disabled, correction input produces no output:
		// the next packet after it is the response to the MON-VER poll.
		if _, err := w.Write(rtcm1230); err != nil {
			t.Fatalf("write: %v", err)
		}
		if _, err := w.Write(ubxbin.Poll(ubxbin.MonVerID)); err != nil {
			t.Fatalf("write: %v", err)
		}
		pkt := await(t, ch, "MON-VER", func(p scan.Packet) bool { return p.Format == ubx.PacketFormat })
		if mid := ubxbin.PacketMsgId(pkt.Data); mid != ubxbin.MonVerID {
			t.Errorf("got %v while RXM-COR disabled, want MON-VER", mid)
		}

		sendMsg(t, w, valsetMsg(ubxbin.CfgValsetLayerRAM,
			ucv.Item{Key: ucv.KUbxRxmCor.KeyU(ucv.UART1).Key(), Value: 1}))
		if !ackResult(t, ch, ubxbin.CfgValsetID) {
			t.Fatalf("VALSET NAKed")
		}
		for _, tc := range tests {
			if _, err := w.Write(tc.in); err != nil {
				t.Fatalf("%s: write: %v", tc.name, err)
			}
			m, err := ubxbin.ParseMsg(await(t, ch, "RXM-COR", isUbx(ubxbin.RxmCorID)).Data)
			if err != nil {
				t.Fatalf("%s: parse RXM-COR: %v", tc.name, err)
			}
			if !reflect.DeepEqual(m, tc.expect) {
				t.Errorf("%s:\ngot  %+v\nwant %+v", tc.name, m, tc.expect)
			}
		}
	})
}
