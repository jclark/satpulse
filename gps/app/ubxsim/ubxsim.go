// Package ubxsim implements a hardware-free fake u-blox receiver: it
// speaks UBX over a byte stream, answers CFG-VALGET/VALSET/VALDEL and
// MON-VER polls with the ACK/NAK semantics of the interface description,
// and emits periodic nav messages by replaying a recorded packet log
// gated by its own MSGOUT configuration. It exists to smoke-test
// configuration wiring end to end without GPS hardware (#362); it is
// explicitly not a model of the config engine's semantics.
//
// Two independent engines feed one output stream: the NAV engine
// (nav.go) replays the personality's message bank, and the config engine
// (cfgdb.go) answers configuration messages. Both enqueue whole packets
// to a single writer goroutine that owns the stream; on a UART port it
// drips the queued bytes out at the configured baud rate, so a large
// packet occupies the line for its transmission time instead of landing
// in one burst followed by silence, and a config response never splices
// into a replayed frame. The byte stream is an io.ReadWriter: a pty for
// black-box use, an in-process pipe in unit tests.
package ubxsim

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"time"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/internal/rtcm"
	"github.com/jclark/satpulse/gps/internal/spartn"
	"github.com/jclark/satpulse/gps/internal/ubx"
	"github.com/jclark/satpulse/gps/lib/rtcmbin"
	"github.com/jclark/satpulse/gps/lib/spartnbin"
	"github.com/jclark/satpulse/gps/lib/ubxbin"
	ucv "github.com/jclark/satpulse/gps/lib/ubxcfgval"
	"github.com/jclark/satpulse/gps/scan"
)

// Options configures a Sim.
type Options struct {
	// Port is the receiver port the simulator pretends to be, selecting
	// the MSGOUT keys that gate nav output and the UART baud-rate key
	// that paces it. The zero value means UART1; simulating the I2C
	// port is not supported.
	Port   ucv.Port
	Logger *slog.Logger // nil discards logs
}

// Sim is a simulator instance for one personality.
type Sim struct {
	p    *Personality
	opts Options
}

// New returns a Sim for the given personality.
func New(p *Personality, opts Options) *Sim {
	if opts.Port == ucv.I2C {
		opts.Port = ucv.UART1
	}
	if opts.Logger == nil {
		opts.Logger = slog.New(slog.DiscardHandler)
	}
	return &Sim{p: p, opts: opts}
}

// Run serves the simulator over rw with fresh configuration state (RAM
// seeded from the personality defaults, BBR and Flash empty). It returns
// when reading from rw fails; cancelling ctx stops the NAV engine but
// unblocks neither a pending read nor a pending write, so to stop the
// simulator the caller closes rw (or its peer) in both directions. A pty
// master does that with one Close; separate pipes need both ends closed.
func (s *Sim) Run(ctx context.Context, rw io.ReadWriter) error {
	db := newCfgDB(s.p.Defaults)
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	w := newWriter(rw, db, s.opts.Port)
	nav := &navEngine{db: db, port: s.opts.Port, epochs: s.p.Epochs, w: w, lg: s.opts.Logger}
	var wg sync.WaitGroup
	wg.Go(func() { nav.run(ctx) })
	// The writer cancels on a fatal write error so blocked senders unwind.
	wg.Go(func() { defer cancel(); w.run(ctx) })
	err := s.readLoop(ctx, db, w, rw)
	cancel()
	wg.Wait()
	return err
}

// readLoop demultiplexes the input stream: configuration messages and
// the MON-VER poll are handled, correction input (RTCM or SPARTN) is
// answered with UBX-RXM-COR, everything else is ignored.
func (s *Sim) readLoop(ctx context.Context, db *cfgDB, w *writer, r io.Reader) error {
	sc := scan.New(r, 4096, []gpsprot.PacketFormat{ubx.PacketFormat, rtcm.PacketFormat, spartn.PacketFormat})
	for {
		pkt, err := sc.Scan()
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
		if pkt.Format == nil || !pkt.ChecksumValid {
			continue
		}
		if pkt.Format == ubx.PacketFormat {
			err = s.handleMsg(ctx, db, w, pkt.Data)
		} else {
			err = s.corInput(ctx, db, w, pkt)
		}
		if err != nil {
			return err
		}
	}
}

// handleMsg processes one received UBX packet. Write errors are
// returned; anything wrong with the packet itself is answered on the
// wire (NAK) per the interface description's acknowledgement rule.
func (s *Sim) handleMsg(ctx context.Context, db *cfgDB, w *writer, data string) error {
	lg := s.opts.Logger
	mid := ubxbin.PacketMsgId(data)
	switch mid {
	case ubxbin.MonVerID:
		if len(data) == ubxbin.PacketMinLen {
			lg.Debug("MON-VER poll")
			return w.send(ctx, s.p.MonVer)
		}
		return nil
	case ubxbin.MonGnssID:
		if len(data) == ubxbin.PacketMinLen && s.p.MonGnss != nil {
			lg.Debug("MON-GNSS poll")
			return w.send(ctx, s.p.MonGnss)
		}
		return nil
	case ubxbin.MonCommsID:
		if len(data) == ubxbin.PacketMinLen {
			lg.Debug("MON-COMMS poll")
			return s.writeMsg(ctx, w, s.monComms())
		}
		return nil
	case ubxbin.CfgValgetID:
		m, err := ubxbin.ParseMsg(data)
		if err != nil {
			return s.writeAckNak(ctx, w, mid, false)
		}
		resp, ok := db.valget(m.(*ubxbin.CfgValget))
		lg.Debug("CFG-VALGET", "ack", ok)
		if ok {
			if err := s.writeMsg(ctx, w, resp); err != nil {
				return err
			}
		}
		return s.writeAckNak(ctx, w, mid, ok)
	case ubxbin.CfgValsetID:
		ok := false
		if m, err := ubxbin.ParseMsg(data); err == nil {
			ok = db.valset(m.(*ubxbin.CfgValset))
		}
		lg.Debug("CFG-VALSET", "ack", ok)
		return s.writeAckNak(ctx, w, mid, ok)
	case ubxbin.CfgValdelID:
		ok := false
		if m, err := ubxbin.ParseMsg(data); err == nil {
			ok = db.valdel(m.(*ubxbin.CfgValdel))
		}
		lg.Debug("CFG-VALDEL", "ack", ok)
		return s.writeAckNak(ctx, w, mid, ok)
	case ubxbin.CfgCfgID:
		ok := false
		if m, err := ubxbin.ParseMsg(data); err == nil {
			ok = db.cfgcfg(m.(*ubxbin.CfgCfg))
		}
		lg.Debug("CFG-CFG", "ack", ok)
		return s.writeAckNak(ctx, w, mid, ok)
	}
	if mid.Ackable() {
		lg.Warn("NAK for unhandled CFG message", "msg", mid.String())
		return s.writeAckNak(ctx, w, mid, false)
	}
	return nil
}

// corInput answers a differential correction input message (RTCM or
// SPARTN, already checksum-validated by the scan layer) with a
// synthesized UBX-RXM-COR, which a real receiver outputs upon
// successful parsing of a correction input message. Emission is gated
// by the RXM-COR MSGOUT key like any output message. The report is
// error-free and used, exercising the positive path of satpulse's
// correction reporting; the correction ID is the DF003 reference
// station ID for the RTCM messages that carry one, else 0xffff, per
// the interface description.
func (s *Sim) corInput(ctx context.Context, db *cfgDB, w *writer, pkt scan.Packet) error {
	if db.ramUint(ucv.KUbxRxmCor.KeyU(s.opts.Port).Key()) == 0 {
		return nil
	}
	m := &ubxbin.RxmCor{Version: 1}
	si := ubxbin.RxmCorErrStatusErrorFree | ubxbin.RxmCorMsgUsedUsed |
		ubxbin.RxmCorMsgTypeValid | ubxbin.RxmCorMsgInputHandle
	corrID := uint16(0xffff)
	if pkt.Format == rtcm.PacketFormat {
		si |= ubxbin.RxmCorProtocolRTCM3 | ubxbin.RxmCorMsgEncryptedNotEncrypted
		mt, sub, hasSub := rtcmbin.ExtractMsgTypeSubtype(pkt.Data)
		m.MsgType = uint16(mt)
		if hasSub {
			m.MsgSubType = sub
			si |= ubxbin.RxmCorMsgSubTypeValid
		}
		if id, ok := rtcmbin.ReferenceStationID(pkt.Data); ok {
			corrID = id
		}
	} else {
		b := []byte(pkt.Data)
		si |= ubxbin.RxmCorProtocolSPARTN | ubxbin.RxmCorMsgSubTypeValid
		if spartnbin.EAF(b) {
			si |= ubxbin.RxmCorMsgEncryptedEncrypted
		} else {
			si |= ubxbin.RxmCorMsgEncryptedNotEncrypted
		}
		m.MsgType = uint16(spartnbin.Type(b))
		m.MsgSubType = uint16(spartnbin.Subtype(b))
	}
	m.StatusInfo = si | ubxbin.RxmCorStatusInfo(corrID)<<9
	s.opts.Logger.Debug("RXM-COR", "tag", pkt.Format.Tag(), "msgType", m.MsgType)
	return s.writeMsg(ctx, w, m)
}

// monComms synthesizes the MON-COMMS poll response the Configurator
// uses to discover the active receiver port: the txErrors output-port
// field and a single port block both report the simulated port.
func (s *Sim) monComms() *ubxbin.MonComms {
	port := s.opts.Port
	return &ubxbin.MonComms{
		MonCommsFixed: ubxbin.MonCommsFixed{
			NPorts:   1,
			TxErrors: ubxbin.MonCommsTxErrors(byte(port+1) << 2),
			ProtIds:  [4]byte{0, 1, 5, 0xff},
		},
		Ports: []ubxbin.MonCommsPort{{PortID: ubxbin.MonCommsPortID(uint16(port) << 8)}},
	}
}

func (s *Sim) writeAckNak(ctx context.Context, w *writer, mid ubxbin.MsgID, ack bool) error {
	if ack {
		return s.writeMsg(ctx, w, &ubxbin.AckAck{MsgID: mid})
	}
	return s.writeMsg(ctx, w, &ubxbin.AckNak{MsgID: mid})
}

func (s *Sim) writeMsg(ctx context.Context, w *writer, m ubxbin.Msg) error {
	pkt, err := ubxbin.Serialize(m)
	if err != nil {
		panic(err)
	}
	return w.send(ctx, pkt)
}

// tickInterval is how often the paced writer wakes to drip queued bytes
// onto the stream. It is a fidelity knob, not a rate: the bytes written
// per tick are derived from elapsed time and the baud rate, so a smaller
// interval only smooths the drip. It is kept well under gpsio's 100ms
// read timeout so no within-burst gap is ever read as line idle.
const tickInterval = time.Millisecond

// bitsPerByte is the 8N1 frame size (8 data bits plus start and stop)
// that turns a baud rate into a byte rate.
const bitsPerByte = 10

// accumPerByte is the bytes-due accumulator increment for one byte. The
// accumulator adds baud (bits/s) times elapsed nanoseconds each tick, so
// one byte becomes due per bitsPerByte * 1e9 baud-nanoseconds.
const accumPerByte int64 = bitsPerByte * int64(time.Second)

// txQueueDepth bounds the packet queue between the engines and the
// writer: it is the simulated transmit buffer. A full queue blocks the
// sender, the smoke-depth analogue of tx-buffer backpressure (real
// firmware would drop and count the overflow; we do not model that).
const txQueueDepth = 16

// writer owns the output stream. The engines enqueue whole packets with
// send; the writer goroutine (run) is the sole writer, so packets never
// interleave. On a UART port it paces the queued bytes onto the line at
// the configured baud rate; other ports are written unpaced.
type writer struct {
	out  io.Writer
	db   *cfgDB
	port ucv.Port
	ch   chan []byte
}

func newWriter(out io.Writer, db *cfgDB, port ucv.Port) *writer {
	return &writer{out: out, db: db, port: port, ch: make(chan []byte, txQueueDepth)}
}

// send enqueues a whole packet for the writer, blocking while the queue
// is full, and returns ctx.Err() if the simulator is shutting down.
func (w *writer) send(ctx context.Context, p []byte) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case w.ch <- p:
		return nil
	}
}

// run drains the queue until ctx is cancelled or a write fails, pacing a
// UART port and passing other ports straight through.
func (w *writer) run(ctx context.Context) {
	if baudKey, ok := uartBaudKey(w.port); ok {
		w.pace(ctx, baudKey)
	} else {
		w.passthrough(ctx)
	}
}

// passthrough writes each queued packet the moment it arrives, the
// unpaced burst shape of a non-UART port (USB, SPI).
func (w *writer) passthrough(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case p := <-w.ch:
			if _, err := w.out.Write(p); err != nil {
				return
			}
		}
	}
}

// pace drips the queued bytes onto the stream at the configured line
// rate. Each tick it credits a bytes-due accumulator with elapsed time
// times the live baud rate and writes that many bytes, pulling further
// packets from the queue to fill the tick's budget so a burst streams
// without gaps. Reading the baud key live lets a mid-session CFG-VALSET
// of the baud rate change the drip rate. The ticker runs only while
// bytes are queued, so an idle port neither wakes nor accrues credit.
func (w *writer) pace(ctx context.Context, baudKey ucv.Key) {
	var buf []byte
	var num int64
	var last time.Time
	var ticker *time.Ticker
	var tick <-chan time.Time
	stop := func() {
		if ticker != nil {
			ticker.Stop()
			ticker, tick = nil, nil
		}
	}
	defer stop()
	for {
		var in <-chan []byte
		if tick == nil {
			in = w.ch // accept the next burst only while idle
		}
		select {
		case <-ctx.Done():
			return
		case p := <-in:
			buf, num, last = p, 0, time.Now()
			ticker = time.NewTicker(tickInterval)
			tick = ticker.C
		case now := <-tick:
			budget, unlimited := 0, false
			if baud := w.db.ramUint(baudKey); baud != 0 {
				num += now.Sub(last).Nanoseconds() * int64(baud)
				last = now
				budget = int(num / accumPerByte)
			} else {
				unlimited = true // baud 0: flush, do not stall the queue
			}
			idle := false
			for unlimited || budget > 0 {
				if len(buf) == 0 {
					select {
					case p := <-w.ch:
						buf = p
					default:
						idle = true
					}
					if idle {
						break
					}
				}
				n := len(buf)
				if !unlimited && n > budget {
					n = budget
				}
				if _, err := w.out.Write(buf[:n]); err != nil {
					return
				}
				buf = buf[n:]
				if !unlimited {
					budget -= n
					num -= int64(n) * accumPerByte
				}
			}
			if idle {
				num = 0
				stop()
			}
		}
	}
}

// uartBaudKey returns the baud-rate config key that paces output on a
// UART port and true, or false for a port that is not paced.
func uartBaudKey(port ucv.Port) (ucv.Key, bool) {
	switch port {
	case ucv.UART1:
		return ucv.KUart1Baudrate.Key(), true
	case ucv.UART2:
		return ucv.KUart2Baudrate.Key(), true
	}
	return 0, false
}
