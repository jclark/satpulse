package ubxsim

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"reflect"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/jclark/satpulse/gps/app/gpscfg"
	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/gpsreg"
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
	dflt[ucv.KUart1outprotUbx.Key()] = 1
	dflt[ucv.KUart1outprotNmea.Key()] = 1
	epochs := make([][]Pkt, 60)
	for i := range epochs {
		epochs[i] = []Pkt{
			{KeyM: ucv.KUbxNavSat, Tag: ubx.Tag, Data: testNavSat()},
			{KeyM: ucv.KNmeaIdGga, Tag: nmea.Tag, Data: testGga()},
		}
	}
	return &Personality{MonVer: testMonVer(), Defaults: dflt, Epochs: epochs}
}

type rwPair struct {
	io.Reader
	io.Writer
}

// simConn runs a Sim over in-process pipes and returns the test-side
// write end and a channel of packets the simulator emits.
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
	// Close both ends before waiting: testWrite unblocks the read loop,
	// testRead a writer parked in simWrite.Write, which watches no context.
	t.Cleanup(func() {
		testWrite.Close()
		testRead.Close()
		if err := <-done; err != nil {
			t.Errorf("sim.Run: %v", err)
		}
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

// TestNavOutProt checks that the bank is gated by the port's OUTPROT key
// as well as by each message's MSGOUT key. Clearing OUTPROT-NMEA is how
// the Configurator turns NMEA off -- it leaves the per-message rates
// alone -- so a simulator that gated on MSGOUT only would keep sending
// GGA and the daemon's NMEA-off config would look like it never landed.
func TestNavOutProt(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		p := testPersonality()
		navSat := ucv.KUbxNavSat.KeyU(ucv.UART1).Key()
		nmeaOut := ucv.KUart1outprotNmea.Key()
		w, ch := simConn(t, p)

		// The defaults leave the port's NMEA output on, and GGA's rate set.
		await(t, ch, "GGA", isNmea("GGA"))

		// NAV-SAT on, giving the epochs that follow a clock.
		sendMsg(t, w, valsetMsg(ubxbin.CfgValsetLayerRAM, ucv.Item{Key: navSat, Value: 1}))
		if !ackResult(t, ch, ubxbin.CfgValsetID) {
			t.Fatalf("VALSET NAV-SAT NAKed")
		}

		// With the port's NMEA output off, no NMEA is sent even though
		// GGA's own rate is still 1. The first NAV-SAT drains the epoch
		// already queued when the VALSET landed.
		sendMsg(t, w, valsetMsg(ubxbin.CfgValsetLayerRAM, ucv.Item{Key: nmeaOut, Value: 0}))
		if !ackResult(t, ch, ubxbin.CfgValsetID) {
			t.Fatalf("VALSET OUTPROT-NMEA off NAKed")
		}
		await(t, ch, "NAV-SAT", isUbx(ubxbin.NavSatID))
		for range 3 {
			await(t, ch, "NAV-SAT", func(p scan.Packet) bool {
				if isNmea("GGA")(p) {
					t.Fatalf("GGA sent with the port's NMEA output disabled")
				}
				return isUbx(ubxbin.NavSatID)(p)
			})
		}

		// Turning the port's NMEA output back on brings GGA back.
		sendMsg(t, w, valsetMsg(ubxbin.CfgValsetLayerRAM, ucv.Item{Key: nmeaOut, Value: 1}))
		if !ackResult(t, ch, ubxbin.CfgValsetID) {
			t.Fatalf("VALSET OUTPROT-NMEA on NAKed")
		}
		await(t, ch, "GGA", isNmea("GGA"))
	})
}

func sendRst(t *testing.T, w io.Writer, mode ubxbin.CfgRstResetMode) {
	t.Helper()
	sendMsg(t, w, &ubxbin.CfgRst{NavBbrMask: ubxbin.CfgRstNavBbrColdStart, ResetMode: mode})
}

func isAckFor(p scan.Packet, mid ubxbin.MsgID) bool {
	if p.Format != ubx.PacketFormat {
		return false
	}
	pmid := ubxbin.PacketMsgId(p.Data)
	if pmid != ubxbin.AckAckID && pmid != ubxbin.AckNakID {
		return false
	}
	return ubxbin.AckMsgID(p.Data) == mid
}

// valgetRAM polls the RAM layer for keys over the pipe and returns the
// response values by key, proving the simulator still serves. It fails
// the test if a CFG-RST is ever acknowledged while the response is
// awaited.
func valgetRAM(t *testing.T, w io.Writer, ch <-chan scan.Packet, keys ...ucv.Key) map[ucv.Key]uint64 {
	t.Helper()
	sendMsg(t, w, valgetPoll(ubxbin.CfgValgetLayerRAM, 0, keys...))
	resp := await(t, ch, "CFG-VALGET response", func(p scan.Packet) bool {
		if isAckFor(p, ubxbin.CfgRstID) {
			t.Fatalf("CFG-RST was acknowledged")
		}
		return isUbx(ubxbin.CfgValgetID)(p)
	})
	m, err := ubxbin.ParseMsg(resp.Data)
	if err != nil {
		t.Fatalf("parse CFG-VALGET: %v", err)
	}
	items, err := ucv.UnmarshalItems(m.(*ubxbin.CfgValget).CfgData)
	if err != nil {
		t.Fatalf("unmarshal items: %v", err)
	}
	out := make(map[ucv.Key]uint64, len(items))
	for _, it := range items {
		out[it.Key] = it.Value
	}
	return out
}

// TestCfgRst checks the reboot semantics of a hardware-reset CFG-RST: it
// is never acknowledged, it rebuilds RAM from the layers below so an
// unsaved change is lost while a saved one survives, and it restarts the
// nav engine with the simulator still serving. A GNSS-only reset mode is
// not a reboot and leaves the RAM configuration alone.
func TestCfgRst(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		p := testPersonality()
		navSat := ucv.KUbxNavSat.KeyU(ucv.UART1).Key()
		nmeaOut := ucv.KUart1outprotNmea.Key()
		w, ch := simConn(t, p)
		await(t, ch, "GGA", isNmea("GGA"))

		// Save navSat=1 to RAM and Flash, giving the epochs a clock and a
		// value that must survive a reboot.
		sendMsg(t, w, valsetMsg(ubxbin.CfgValsetLayerRAM|ubxbin.CfgValsetLayerFlash,
			ucv.Item{Key: navSat, Value: 1}))
		if !ackResult(t, ch, ubxbin.CfgValsetID) {
			t.Fatalf("VALSET navSat NAKed")
		}

		// Turn the port's NMEA output off in RAM only (unsaved): GGA stops.
		sendMsg(t, w, valsetMsg(ubxbin.CfgValsetLayerRAM, ucv.Item{Key: nmeaOut, Value: 0}))
		if !ackResult(t, ch, ubxbin.CfgValsetID) {
			t.Fatalf("VALSET nmeaOut off NAKed")
		}
		await(t, ch, "NAV-SAT", isUbx(ubxbin.NavSatID)) // drain the queued epoch
		for range 3 {
			await(t, ch, "NAV-SAT", func(p scan.Packet) bool {
				if isNmea("GGA")(p) {
					t.Fatalf("GGA sent with the port's NMEA output disabled")
				}
				return isUbx(ubxbin.NavSatID)(p)
			})
		}

		// A hardware-reset CFG-RST reboots: RAM is rebuilt (NMEA output
		// back to its default-on) and the nav engine restarts, so GGA
		// resumes. No ACK/NAK for CFG-RST appears while we wait for it.
		sendRst(t, w, ubxbin.CfgRstResetModeHardwareResetImmediately)
		await(t, ch, "GGA", func(p scan.Packet) bool {
			if isAckFor(p, ubxbin.CfgRstID) {
				t.Fatalf("CFG-RST was acknowledged")
			}
			return isNmea("GGA")(p)
		})

		// The saved navSat survived; the unsaved nmeaOut change was lost.
		vals := valgetRAM(t, w, ch, navSat, nmeaOut)
		if vals[navSat] != 1 {
			t.Errorf("after reboot navSat=%d, want saved 1", vals[navSat])
		}
		if vals[nmeaOut] != 1 {
			t.Errorf("after reboot nmeaOut=%d, want default 1", vals[nmeaOut])
		}

		// A GNSS-only reset mode is not a reboot: an unsaved RAM change to
		// nmeaOut survives it.
		sendMsg(t, w, valsetMsg(ubxbin.CfgValsetLayerRAM, ucv.Item{Key: nmeaOut, Value: 0}))
		if !ackResult(t, ch, ubxbin.CfgValsetID) {
			t.Fatalf("VALSET nmeaOut off NAKed")
		}
		sendRst(t, w, ubxbin.CfgRstResetModeControlledGnssStop)
		if v := valgetRAM(t, w, ch, nmeaOut)[nmeaOut]; v != 0 {
			t.Errorf("after GNSS-stop reset nmeaOut=%d, want unchanged 0", v)
		}
	})
}

// TestCfgRstFactory checks the factory-reset sequence the workbench
// sends: a CFG-CFG that clears the saved layers followed by a
// hardware-reset CFG-RST. Without the CFG-RST reboot the RAM would keep
// its current value; with it, RAM rebuilds from Default alone.
func TestCfgRstFactory(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		p := testPersonality()
		navSat := ucv.KUbxNavSat.KeyU(ucv.UART1).Key()
		w, ch := simConn(t, p)
		await(t, ch, "GGA", isNmea("GGA"))

		// Put navSat=1 into RAM and both non-volatile layers.
		sendMsg(t, w, valsetMsg(ubxbin.CfgValsetLayerRAM|ubxbin.CfgValsetLayerBBR|ubxbin.CfgValsetLayerFlash,
			ucv.Item{Key: navSat, Value: 1}))
		if !ackResult(t, ch, ubxbin.CfgValsetID) {
			t.Fatalf("VALSET NAKed")
		}

		// Factory reset: clear the saved layers, then reboot.
		sendMsg(t, w, &ubxbin.CfgCfg{CfgCfgFixed: ubxbin.CfgCfgFixed{ClearMask: ubxbin.CfgCfgSectionMaskAll}})
		if !ackResult(t, ch, ubxbin.CfgCfgID) {
			t.Fatalf("CFG-CFG clear NAKed")
		}
		sendRst(t, w, ubxbin.CfgRstResetModeHardwareResetImmediately)

		if v := valgetRAM(t, w, ch, navSat)[navSat]; v != 0 {
			t.Errorf("after factory reset navSat=%d, want default 0", v)
		}
	})
}

// recWriter records the time and size of each Write so a test can
// observe how the paced writer meters a packet onto the stream.
type recWriter struct {
	mu   sync.Mutex
	recs []writeRec
}

type writeRec struct {
	at time.Time
	n  int
}

func (w *recWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	w.recs = append(w.recs, writeRec{time.Now(), len(p)})
	w.mu.Unlock()
	return len(p), nil
}

func (w *recWriter) snapshot() []writeRec {
	w.mu.Lock()
	defer w.mu.Unlock()
	return append([]writeRec(nil), w.recs...)
}

// TestWriterPacing checks that a UART port meters a large packet onto
// the stream over its transmission time rather than writing it in one
// burst, so no within-burst gap the reader sees approaches the idle
// threshold. It exercises the writer directly, without the engines.
func TestWriterPacing(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		const baud = 38400
		db := newCfgDB(ucv.Map{ucv.KUart1Baudrate.Key(): baud})
		rec := &recWriter{}
		w := newWriter(rec, db, ucv.UART1)
		ctx, cancel := context.WithCancel(t.Context())
		done := make(chan struct{})
		go func() { w.run(ctx); close(done) }()

		big, err := ubxbin.PackMsg(ubxbin.NavSatID, make([]byte, 700))
		if err != nil {
			t.Fatalf("pack: %v", err)
		}
		want := time.Duration(len(big)*bitsPerByte) * time.Second / baud
		start := time.Now()
		if err := w.send(ctx, big); err != nil {
			t.Fatalf("send: %v", err)
		}
		time.Sleep(want + 20*tickInterval) // let the writer drip it all out
		cancel()
		<-done

		recs := rec.snapshot()
		total := 0
		for _, r := range recs {
			total += r.n
		}
		if total != len(big) {
			t.Errorf("wrote %d bytes, want %d", total, len(big))
		}
		if len(recs) < 2 {
			t.Fatalf("packet emitted in %d write(s), want a metered drip", len(recs))
		}
		for i := 1; i < len(recs); i++ {
			if gap := recs[i].at.Sub(recs[i-1].at); gap > tickInterval {
				t.Errorf("within-burst gap %v exceeds tick interval %v", gap, tickInterval)
			}
		}
		if span := recs[len(recs)-1].at.Sub(start); span < want-2*tickInterval || span > want+2*tickInterval {
			t.Errorf("drip spanned %v, want ~%v (a packet's transmission time)", span, want)
		}
	})
}

// TestWriterReboot checks that a simulated reboot drops the packets a
// slow UART port has queued (and the one it is dripping) under the
// pre-reset config instead of emitting them ahead of the post-reset
// stream: a real receiver reset abandons its in-flight output. It
// exercises the writer directly, without the engines.
func TestWriterReboot(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		// A high baud so that, without the fix, the whole stale burst would
		// drip out inside the sleep below and be observed.
		const baud = 921600
		db := newCfgDB(ucv.Map{ucv.KUart1Baudrate.Key(): baud})
		var buf bytes.Buffer
		w := newWriter(&buf, db, ucv.UART1)
		ctx, cancel := context.WithCancel(t.Context())
		done := make(chan struct{})
		go func() { w.run(ctx); close(done) }()

		// Fill the queue with pre-reset nav output. The first packet is
		// large so the slow line is still dripping it when the reset lands.
		stale, err := ubxbin.PackMsg(ubxbin.NavSatID, make([]byte, 400))
		if err != nil {
			t.Fatalf("pack stale: %v", err)
		}
		for range txQueueDepth {
			if err := w.send(ctx, stale); err != nil {
				t.Fatalf("send stale: %v", err)
			}
		}
		synctest.Wait() // let the writer take the first packet and park on its tick

		// Reboot, then enqueue the post-reset marker under the new
		// generation. The stale packets, all under the old generation, must
		// not reach the wire.
		w.reboot()
		fresh, err := ubxbin.PackMsg(ubxbin.MonVerID, []byte("post-reset"))
		if err != nil {
			t.Fatalf("pack fresh: %v", err)
		}
		if err := w.send(ctx, fresh); err != nil {
			t.Fatalf("send fresh: %v", err)
		}
		time.Sleep(100 * time.Millisecond) // let the fresh packet drip out
		cancel()
		<-done

		sc := scan.New(bytes.NewReader(buf.Bytes()), 4096, []gpsprot.PacketFormat{ubx.PacketFormat})
		gotFresh := false
		for {
			pkt, err := sc.Scan()
			if err != nil {
				break
			}
			if pkt.Format == nil || !pkt.ChecksumValid {
				continue
			}
			switch ubxbin.PacketMsgId(pkt.Data) {
			case ubxbin.NavSatID:
				t.Errorf("stale NAV-SAT emitted after reboot")
			case ubxbin.MonVerID:
				gotFresh = true
			}
		}
		if !gotFresh {
			t.Errorf("post-reset MON-VER not emitted")
		}
	})
}

// TestNavRebootMidBurst checks that a reboot landing while the nav engine
// is mid-way through an epoch burst does not let the tail of that burst
// precede the restarted epoch 0: the run is bound to the generation live
// at its start, so packets the engine sends after the reboot bumped the
// generation are still dropped. The bank's message stays enabled across
// the reboot (it is in the defaults), which is exactly the case the queue
// flush alone cannot catch.
func TestNavRebootMidBurst(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		db := newCfgDB(ucv.Map{
			ucv.KUart1Baudrate.Key():             921600,
			ucv.KUart1outprotUbx.Key():           1,
			ucv.KUbxNavSat.KeyU(ucv.UART1).Key(): 1,
		})
		var buf bytes.Buffer
		w := newWriter(&buf, db, ucv.UART1)
		// One epoch, deep enough that the engine is still blocked sending
		// its burst when the reboot lands. Each packet carries its index.
		burst := txQueueDepth + 8
		epoch := make([]Pkt, burst)
		for i := range epoch {
			data, err := ubxbin.PackMsg(ubxbin.NavSatID, append([]byte{byte(i)}, make([]byte, 199)...))
			if err != nil {
				t.Fatalf("pack: %v", err)
			}
			epoch[i] = Pkt{KeyM: ucv.KUbxNavSat, Tag: ubx.Tag, Data: data}
		}
		nav := &navEngine{db: db, port: ucv.UART1, epochs: [][]Pkt{epoch}, w: w, lg: slog.New(slog.DiscardHandler), reboot: make(chan struct{}, 1)}
		ctx, cancel := context.WithCancel(t.Context())
		done := make(chan struct{})
		go func() { w.run(ctx); close(done) }()
		navDone := make(chan struct{})
		go func() { nav.run(ctx); close(navDone) }()
		synctest.Wait() // the queue is full, the engine blocked mid-burst

		mark := buf.Len()
		db.reboot()
		w.reboot()
		nav.restart()
		time.Sleep(2 * time.Second) // the restarted epoch 0 drips out fully
		cancel()
		<-done
		<-navDone

		// Everything on the wire after the reboot must be the restarted
		// epoch from its first packet on; a pre-reset tail would show as
		// indices out of order at the front.
		sc := scan.New(bytes.NewReader(buf.Bytes()[mark:]), 4096, []gpsprot.PacketFormat{ubx.PacketFormat})
		var got []int
		for {
			pkt, err := sc.Scan()
			if err != nil {
				break
			}
			if pkt.Format == nil || !pkt.ChecksumValid || ubxbin.PacketMsgId(pkt.Data) != ubxbin.NavSatID {
				continue
			}
			got = append(got, int(pkt.Data[6])) // first payload byte: the index
		}
		if len(got) == 0 || got[0] != 0 {
			t.Fatalf("first post-reset NAV-SAT index = %v, want 0 (pre-reset tail leaked)", got)
		}
		for i, idx := range got {
			if idx != i {
				t.Fatalf("post-reset NAV-SAT indices %v, want 0..%d in order", got, burst-1)
			}
		}
		if len(got) != burst {
			t.Errorf("post-reset NAV-SAT count = %d, want %d", len(got), burst)
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

// TestCfgPrt checks that the CFG-PRT poll response is synthesized from
// the config database: PortID from the polled port, baud from its
// baud-rate key (UART only), and the protocol masks from its
// INPROT/OUTPROT boolean keys.
func TestCfgPrt(t *testing.T) {
	tests := []struct {
		name   string
		dflt   ucv.Map
		port   ucv.Port
		expect *ubxbin.CfgPrt
	}{
		{
			name: "UART1 all protocols",
			dflt: ucv.Map{
				ucv.KUart1Baudrate.Key():      115200,
				ucv.KUart1inprotUbx.Key():     1,
				ucv.KUart1inprotNmea.Key():    1,
				ucv.KUart1inprotRtcm3x.Key():  1,
				ucv.KUart1outprotUbx.Key():    1,
				ucv.KUart1outprotNmea.Key():   1,
				ucv.KUart1outprotRtcm3x.Key(): 1,
			},
			port: ucv.UART1,
			expect: &ubxbin.CfgPrt{
				PortID:       ubxbin.PortUART1,
				Mode:         ubxbin.CfgPrtModeCharLen8 | ubxbin.CfgPrtModeParityNone,
				BaudRate:     115200,
				InProtoMask:  ubxbin.CfgPrtProtoUBX | ubxbin.CfgPrtProtoNMEA | ubxbin.CfgPrtProtoRTCM3,
				OutProtoMask: ubxbin.CfgPrtProtoUBX | ubxbin.CfgPrtProtoNMEA | ubxbin.CfgPrtProtoRTCM3,
			},
		},
		{
			name: "UART1 UBX only",
			dflt: ucv.Map{
				ucv.KUart1Baudrate.Key():   38400,
				ucv.KUart1inprotUbx.Key():  1,
				ucv.KUart1outprotUbx.Key(): 1,
			},
			port: ucv.UART1,
			expect: &ubxbin.CfgPrt{
				PortID:       ubxbin.PortUART1,
				Mode:         ubxbin.CfgPrtModeCharLen8 | ubxbin.CfgPrtModeParityNone,
				BaudRate:     38400,
				InProtoMask:  ubxbin.CfgPrtProtoUBX,
				OutProtoMask: ubxbin.CfgPrtProtoUBX,
			},
		},
		{
			name: "USB has no baud rate or UART mode",
			dflt: ucv.Map{
				ucv.KUsbinprotUbx.Key():   1,
				ucv.KUsboutprotUbx.Key():  1,
				ucv.KUsboutprotNmea.Key(): 1,
			},
			port: ucv.USB,
			expect: &ubxbin.CfgPrt{
				PortID:       ubxbin.PortUSB,
				InProtoMask:  ubxbin.CfgPrtProtoUBX,
				OutProtoMask: ubxbin.CfgPrtProtoUBX | ubxbin.CfgPrtProtoNMEA,
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := cfgPrt(newCfgDB(tc.dflt), tc.port)
			if !reflect.DeepEqual(got, tc.expect) {
				t.Errorf("got  %+v\nwant %+v", got, tc.expect)
			}
		})
	}
}

// TestCfgPrtPoll drives the CFG-PRT poll over the pipe: a no-payload
// poll and a 1-byte poll for the simulated port both return the port
// config and an ACK; a 1-byte poll for a port the simulator does not
// model is NAKed with no response.
func TestCfgPrtPoll(t *testing.T) {
	prtPoll := func(payload ...byte) []byte {
		pkt, err := ubxbin.PackMsg(ubxbin.CfgPrtID, payload)
		if err != nil {
			t.Fatalf("pack CFG-PRT poll: %v", err)
		}
		return pkt
	}
	tests := []struct {
		name      string
		poll      []byte
		expectNak bool
	}{
		{name: "no-payload poll", poll: prtPoll()},
		{name: "own-port poll", poll: prtPoll(byte(ucv.UART1))},
		{name: "other-port poll NAKed", poll: prtPoll(byte(ucv.USB)), expectNak: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				w, ch := simConn(t, testPersonality())
				if _, err := w.Write(tc.poll); err != nil {
					t.Fatalf("write: %v", err)
				}
				if tc.expectNak {
					if ackResult(t, ch, ubxbin.CfgPrtID) {
						t.Errorf("poll for unmodelled port ACKed")
					}
					return
				}
				resp := await(t, ch, "CFG-PRT", isUbx(ubxbin.CfgPrtID))
				m, err := ubxbin.ParseMsg(resp.Data)
				if err != nil {
					t.Fatalf("parse CFG-PRT: %v", err)
				}
				if got := m.(*ubxbin.CfgPrt).PortID; got != ubxbin.PortUART1 {
					t.Errorf("CFG-PRT PortID = %d, want UART1", got)
				}
				if !ackResult(t, ch, ubxbin.CfgPrtID) {
					t.Errorf("CFG-PRT poll NAKed")
				}
			})
		})
	}
}

// fakeOutPort adapts an io.Writer (the simulator's input pipe) to the
// gpsio.OutPort gpscfg.Configure writes probes and requests to.
type fakeOutPort struct{ w io.Writer }

func (p *fakeOutPort) Write(b []byte) (int, error) { return p.w.Write(b) }
func (p *fakeOutPort) Buffered() (int, error)      { return 0, nil }
func (p *fakeOutPort) ReadOnly() bool              { return false }
func (p *fakeOutPort) Direct() bool                { return true }

// TestConfiguratorPortDiscovery drives the real Configurator through
// gpscfg.Configure against the shipped F9P personality (protVer 27.50),
// where port discovery goes via a CFG-PRT poll rather than MON-COMMS.
// Getting the port and the enabled signals reduces the run to the
// MON-VER probe, that poll, and valGetSignals' VALGET of the SIGNAL
// group wildcard plus a wildcard for a group the F9P database has no
// keys in -- the poll a real F9P ACKed with only the populated group's
// items (gpshwtest001/019.jsonl). Completing without error and
// reporting UART1 and a signal set proves both paths work against the
// simulator.
func TestConfiguratorPortDiscovery(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		p, err := LoadPersonality("testdata/f9p/f9p-personality.ubx")
		if err != nil {
			t.Fatalf("load personality: %v", err)
		}
		w, ch := simConn(t, p)
		target := gpsprot.NewConfigTarget()
		target.Get = gpsprot.PropIDPort | gpsprot.PropIDSignalsEnabled
		rslt, err := gpscfg.Configure(t.Context(), slog.New(slog.DiscardHandler),
			gpsreg.CreatePacketProcessors([]gpsreg.Vendor{gpsreg.VendorUblox}),
			[]gpsprot.ConfigProtocol{ubx.NewConfigProtocol()},
			target, ch, &fakeOutPort{w: w})
		if err != nil {
			t.Fatalf("Configure: %v", err)
		}
		if port, ok := rslt.ConfigProps.GetPort(); !ok || port != "UART1" {
			t.Errorf("discovered port %q (ok=%v), want UART1", port, ok)
		}
		if sigs, ok := rslt.ConfigProps.GetSignalsEnabled(); !ok || sigs == 0 {
			t.Errorf("signals enabled %v (ok=%v), want a non-empty set", sigs, ok)
		}
	})
}
