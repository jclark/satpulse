// Package ubxsim implements a hardware-free fake u-blox receiver: it
// speaks UBX over a byte stream, answers CFG-VALGET/VALSET/VALDEL and
// MON-VER polls with the ACK/NAK semantics of the interface description,
// and emits periodic nav messages by replaying a recorded packet log
// gated by its own MSGOUT configuration. It exists to smoke-test
// configuration wiring end to end without GPS hardware (#362); it is
// explicitly not a model of the config engine's semantics.
//
// Two independent engines are multiplexed onto one output stream: the
// NAV engine (nav.go) replays the personality's message bank, and the
// config engine (cfgdb.go) answers configuration messages. Output is
// written whole packets at a time under a mutex, so a config response is
// never spliced into the middle of a replayed frame. The byte stream is
// an io.ReadWriter: a pty for black-box use, an in-process pipe in unit
// tests.
package ubxsim

import (
	"context"
	"io"
	"log/slog"
	"sync"

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
// cannot unblock a pending read, so to stop the simulator the caller
// closes rw (or its peer).
func (s *Sim) Run(ctx context.Context, rw io.ReadWriter) error {
	db := newCfgDB(s.p.Defaults)
	w := &muxWriter{w: rw}
	nav := &navEngine{db: db, port: s.opts.Port, epochs: s.p.Epochs, w: w, lg: s.opts.Logger}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	var wg sync.WaitGroup
	wg.Go(func() { nav.run(ctx) })
	err := s.readLoop(db, w, rw)
	cancel()
	wg.Wait()
	return err
}

// readLoop demultiplexes the input stream: configuration messages and
// the MON-VER poll are handled, correction input (RTCM or SPARTN) is
// answered with UBX-RXM-COR, everything else is ignored.
func (s *Sim) readLoop(db *cfgDB, w *muxWriter, r io.Reader) error {
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
			err = s.handleMsg(db, w, pkt.Data)
		} else {
			err = s.corInput(db, w, pkt)
		}
		if err != nil {
			return err
		}
	}
}

// handleMsg processes one received UBX packet. Write errors are
// returned; anything wrong with the packet itself is answered on the
// wire (NAK) per the interface description's acknowledgement rule.
func (s *Sim) handleMsg(db *cfgDB, w *muxWriter, data string) error {
	lg := s.opts.Logger
	mid := ubxbin.PacketMsgId(data)
	switch mid {
	case ubxbin.MonVerID:
		if len(data) == ubxbin.PacketMinLen {
			lg.Debug("MON-VER poll")
			return w.writePacket(s.p.MonVer)
		}
		return nil
	case ubxbin.MonGnssID:
		if len(data) == ubxbin.PacketMinLen && s.p.MonGnss != nil {
			lg.Debug("MON-GNSS poll")
			return w.writePacket(s.p.MonGnss)
		}
		return nil
	case ubxbin.MonCommsID:
		if len(data) == ubxbin.PacketMinLen {
			lg.Debug("MON-COMMS poll")
			return s.writeMsg(w, s.monComms())
		}
		return nil
	case ubxbin.CfgValgetID:
		m, err := ubxbin.ParseMsg(data)
		if err != nil {
			return s.writeAckNak(w, mid, false)
		}
		resp, ok := db.valget(m.(*ubxbin.CfgValget))
		lg.Debug("CFG-VALGET", "ack", ok)
		if ok {
			if err := s.writeMsg(w, resp); err != nil {
				return err
			}
		}
		return s.writeAckNak(w, mid, ok)
	case ubxbin.CfgValsetID:
		ok := false
		if m, err := ubxbin.ParseMsg(data); err == nil {
			ok = db.valset(m.(*ubxbin.CfgValset))
		}
		lg.Debug("CFG-VALSET", "ack", ok)
		return s.writeAckNak(w, mid, ok)
	case ubxbin.CfgValdelID:
		ok := false
		if m, err := ubxbin.ParseMsg(data); err == nil {
			ok = db.valdel(m.(*ubxbin.CfgValdel))
		}
		lg.Debug("CFG-VALDEL", "ack", ok)
		return s.writeAckNak(w, mid, ok)
	case ubxbin.CfgCfgID:
		ok := false
		if m, err := ubxbin.ParseMsg(data); err == nil {
			ok = db.cfgcfg(m.(*ubxbin.CfgCfg))
		}
		lg.Debug("CFG-CFG", "ack", ok)
		return s.writeAckNak(w, mid, ok)
	}
	if mid.Ackable() {
		lg.Warn("NAK for unhandled CFG message", "msg", mid.String())
		return s.writeAckNak(w, mid, false)
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
func (s *Sim) corInput(db *cfgDB, w *muxWriter, pkt scan.Packet) error {
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
	return s.writeMsg(w, m)
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

func (s *Sim) writeAckNak(w *muxWriter, mid ubxbin.MsgID, ack bool) error {
	if ack {
		return s.writeMsg(w, &ubxbin.AckAck{MsgID: mid})
	}
	return s.writeMsg(w, &ubxbin.AckNak{MsgID: mid})
}

func (s *Sim) writeMsg(w *muxWriter, m ubxbin.Msg) error {
	pkt, err := ubxbin.Serialize(m)
	if err != nil {
		panic(err)
	}
	return w.writePacket(pkt)
}

// muxWriter serializes whole-packet writes from the config and NAV
// engines onto the shared output stream.
type muxWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func (m *muxWriter) writePacket(p []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, err := m.w.Write(p)
	return err
}
