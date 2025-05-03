package gpscfg

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/jclark/satpulse/internal/gpsio"
	"github.com/jclark/satpulse/internal/gpsprot"
	"github.com/jclark/satpulse/internal/nmea"
	"github.com/jclark/satpulse/internal/rtcm"
	"github.com/jclark/satpulse/internal/scan"
	"github.com/jclark/satpulse/internal/ubx"
	"golang.org/x/exp/maps"
)

type Result struct {
	Version     *ubx.Version
	ConfigProps *gpsprot.ConfigProps
	LeapSecond  *gpsprot.LeapSecondMsg
}

type msgHandler struct {
	gpsprot.DefaultHandler
	lg          *slog.Logger
	packetProcs map[gpsprot.Tag]gpsprot.PacketProcessor
	packetExch  gpsprot.PacketExchanger
	packetCh    <-chan scan.Packet
	msgCount    map[gpsprot.Tag]int
	bad         badCount
	msgIDs      map[gpsprot.Tag]map[string]bool
	leapSecond  *gpsprot.LeapSecondMsg
}

type badCount struct {
	invalidBytes, corruptMsgs, framingErrs int
}

var _ gpsprot.NativeMsgHandler = &msgHandler{}

var ErrNoResponse = errors.New("no response to configuration poll message; not configuring GPS")

func Configure(ctx context.Context, lg *slog.Logger, packetProcs map[gpsprot.Tag]gpsprot.PacketProcessor, target *gpsprot.ConfigTarget, packetCh <-chan scan.Packet, port gpsio.OutPort) (*Result, error) {
	mh := msgHandler{}
	mh.init(lg, packetProcs, packetCh)
	var err error
	if !target.Opts.Detected {
		err = mh.detect(ctx)
	}
	noop := target.NoOp()
	if err != nil {
		// even with NoOp, we want to run detect in order to deal with framing errors
		if noop {
			// there's no point in giving up with NoOp, maybe we can bring it back to life using satpulsetool
			lg.Warn(err.Error())
		} else if target.Opts.ForceProbe {
			lg.Info("no output detected from GPS, but trying to probe GPS anyway")
		} else {
			return nil, err
		}
	}
	if noop {
		// no need to probe
		return &Result{
			ConfigProps: new(gpsprot.ConfigProps),
		}, nil
	}
	// After we have done detection, bad stuff is a cause for concern.
	badStart := mh.bad
	probeOK, err := mh.probe(ctx, mh.packetExch, port)
	if err != nil {
		return nil, err
	}
	badNew := mh.bad.Sub(badStart)
	if badNew.framingErrs > 0 {
		return nil, errors.New("ongoing framing errors reading GPS output (hardware problems?)")
	}
	if badNew.corruptMsgs > 0 {
		return nil, errors.New("ongoing corrupted GPS output (multiple processes reading from serial port?)")
	}
	var cm *gpsprot.ConfigProps
	if probeOK {
		cm, err = mh.configure(ctx, mh.packetExch, target, port)
		if err != nil {
			// XXX try to recover from this
			// provided we have some messages working, we should be OK
			return nil, err
		}
	} else {
		// XXX if msgCount for UBX > 0, but probe failed, then probably we cannot send to the GPS
		return &Result{
			ConfigProps: new(gpsprot.ConfigProps),
		}, ErrNoResponse
	}
	return mh.finish(cm), nil
}

func (mh *msgHandler) init(lg *slog.Logger, packetProcs map[gpsprot.Tag]gpsprot.PacketProcessor, packetCh <-chan scan.Packet) {
	mh.lg = lg
	mh.packetProcs = packetProcs
	mh.packetCh = packetCh
	mh.msgCount = map[gpsprot.Tag]int{}
	mh.msgIDs = map[gpsprot.Tag]map[string]bool{}
	for tag, pp := range packetProcs {
		pp.SetMsgHandler(mh)
		pp.SetNativeMsgHandler(mh)
		if mh.packetExch == nil {
			mh.packetExch = pp.CreatePacketExchanger()
		}
		mh.msgCount[tag] = 0
		mh.msgIDs[tag] = map[string]bool{}
	}
}

func (mh *msgHandler) finish(cm *gpsprot.ConfigProps) *Result {
	lg := mh.lg
	if cm != nil {
		lg.Info("GPS configuration", "cfg", cm)
	}
	if mh.leapSecond != nil {
		lsdStr := mh.leapSecond.Date().Format("2006-01-02")
		lg.Info("leap second information received from GPS", "date", lsdStr, "utcOffBefore", mh.leapSecond.UTCOffBefore, "utcOffAfter", mh.leapSecond.UTCOffAfter)
	}
	// XXX should find cleaner way to do this
	ver := mh.packetExch.(*ubx.PacketExchanger).Version()
	if ver != nil {
		lg.Info("GPS version", "model", ver.Mod, "category", ver.ProductCategory(), "flash", ver.Flash,
			"sw", ver.SW, "hw", ver.HW, "prot", ver.Prot, "gnss", ver.GNSS, "ext", ver.Extensions)
	}
	for tag, msgIDs := range mh.msgIDs {
		if len(msgIDs) > 0 {
			lg.Info("message types received during configuration", "protocol", tag, "msgIDs", maps.Keys(msgIDs))
		}
	}
	return &Result{
		Version:     ver,
		ConfigProps: cm,
		LeapSecond:  mh.leapSecond,
	}
}

func (mh *msgHandler) detect(ctx context.Context) error {
	// Validate that we are receiving data correctly from a GPS.
	// The criteria for this is that we get a NMEA or UBX message with a valid checksum
	// (not necessarily a message that we understand).
	// This stage finishes as soon as we get such a message.
	// I have found that when starting to read from a GPS, there can be invalid bytes to start with:
	// one possible cause of this if that we start reading in the middle of the message;
	// I think there may also be UART issues that cause framing errors when starting to read.
	// We allow 2 seconds if there is no framing error.
	// We allow 15 seconds if we get a framing error.
	timer1Ch := time.After(time.Second * 2)
	timer2Ch := time.After(time.Second * 15)
	for mh.packetCh != nil {
		select {
		case packet, ok := <-mh.packetCh:
			if !ok {
				return mh.packetChClosed(ctx)
			}
			mh.packet(packet)
		case <-timer1Ch:
			timer1Ch = nil
		case <-timer2Ch:
			timer2Ch = nil
		}
		if mh.suitableMessageCount() > 0 || timer2Ch == nil || (timer1Ch == nil && mh.bad.framingErrs == 0) {
			break
		}
	}
	lg := mh.lg
	if mh.suitableMessageCount() == 0 {
		var msg string
		if mh.bad.framingErrs > 0 {
			msg = "framing errors reading GPS output (wrong speed?)"
		} else if mh.msgCount[rtcm.Tag] > 0 {
			msg = "only RTCM messages detected from GPS"
		} else if mh.bad.invalidBytes+mh.bad.corruptMsgs == 0 {
			msg = "no output detected from GPS"
		} else if mh.bad.corruptMsgs > 0 {
			msg = "corrupted GPS output (multiple processes reading from serial port?)"
		} else {
			msg = "cannot parse GPS output"
		}
		lg.Debug("not receiving data from GPS correctly", "bad", mh.bad, "rtcmMsgCount", mh.msgCount[rtcm.Tag])
		return errors.New(msg)
	}
	lg.Info("detected a GPS")

	lg.Debug("received suitable output message from GPS", "msgCount", mh.msgCount, "bad", mh.bad)
	return nil
}

func (mh *msgHandler) packetChClosed(ctx context.Context) error {
	mh.packetCh = nil
	err := ctx.Err()
	if err != nil {
		return err
	}
	return io.ErrUnexpectedEOF
}

func (mh *msgHandler) probe(ctx context.Context, prot gpsprot.PacketExchanger, port gpsio.OutPort) (bool, error) {
	msg := prot.ProbePacket()
	_, err := port.Write(msg)
	if err != nil {
		return false, err
	}
	timerCh := time.After(time.Millisecond * 1500)

	for timerCh != nil {
		select {
		case packet, ok := <-mh.packetCh:
			if !ok {
				return false, mh.packetChClosed(ctx)
			}
			mh.packet(packet)
			if prot.ProbeOK() {
				return true, nil
			}
		case <-timerCh:
			timerCh = nil
		}
	}
	return false, nil
}

func (mh *msgHandler) configure(ctx context.Context, prot gpsprot.PacketExchanger, target *gpsprot.ConfigTarget, port gpsio.OutPort) (*gpsprot.ConfigProps, error) {
	cfgtor, err := prot.Configure(target)
	if err != nil {
		return nil, err
	}
	for {
		req, err := cfgtor.NextRequest()
		if err != nil {
			mh.lg.Warn("GPS configuration failed", "err", err)
			continue
		}
		if req == nil {
			break
		}
		err = mh.doRequest(ctx, cfgtor, req, port)
		if err != nil {
			return nil, err
		}
	}
	return cfgtor.ConfigProps(), nil
}

const maxTries = 3

func (mh *msgHandler) doRequest(ctx context.Context, cfgtor gpsprot.Configurator, req gpsprot.ConfigRequest, port gpsio.OutPort) error {
	try := 0
	pkt := req.Packet()
	for {
		t := time.Now()
		_, err := port.Write(pkt)
		if err != nil {
			return err
		}
		err = mh.waitAfterSend(ctx, cfgtor, req, port, pkt, t)
		if err == nil {
			break
		}
		try++
		if try >= maxTries {
			return err
		}
		if !errors.Is(err, errNoResponse) && !errors.Is(err, errNoAck) {
			return err
		}
		mh.lg.Info("retrying configuration request", "msgID", req.ID())
	}
	return nil
}

const minWaitAfterSend = 10 * time.Millisecond

var errNoResponse = errors.New("no response to configuration poll message")
var errNoAck = errors.New("no ack to configuration message")

func (mh *msgHandler) waitAfterSend(ctx context.Context, cfgtor gpsprot.Configurator, req gpsprot.ConfigRequest, port gpsio.OutPort, pkt []byte, tSend time.Time) error {
	w := max(port.TransmitTime(len(pkt)), minWaitAfterSend)
	awaitingAck := req.Ackable()
	awaitingResp := req.AwaitingResponse(tSend)
	if awaitingAck || awaitingResp {
		w = time.Millisecond * 1500
	}
	reqID := req.ID()
	mh.lg.Debug("sent configuration message", "msgID", reqID, "len", len(pkt))
	timerCh := time.After(w)
	for timerCh != nil {
		select {
		case packet, ok := <-mh.packetCh:
			if !ok {
				return mh.packetChClosed(ctx)
			}
			mh.packet(packet)
			if awaitingResp {
				awaitingResp = req.AwaitingResponse(tSend)
			}
			if awaitingAck {
				ack := cfgtor.FindAck(pkt, tSend)
				if ack != nil {
					if !ack.OK {
						mh.lg.Warn("GPS configuration failed: message rejected", "msgID", reqID)
						return nil
					}
					req.Done()
					mh.lg.Debug("configuration message accepted", "msgID", reqID, "delay", ack.TRead.Sub(tSend).String())
					awaitingAck = false
					if awaitingResp {
						mh.lg.Info("configuration message acknowledged before poll response", "msgID", reqID)
					}
				}
			}
			if !awaitingAck && !awaitingResp {
				return nil
			}
		case <-timerCh:
			timerCh = nil
		}
	}
	if awaitingAck {
		return fmt.Errorf("%w %s", errNoAck, reqID)
	}
	if awaitingResp {
		return fmt.Errorf("%w %s", errNoResponse, reqID)
	}
	return nil
}

func (mh *msgHandler) suitableMessageCount() int {
	return mh.msgCount[ubx.Tag] + mh.msgCount[nmea.Tag]
}

func (mh *msgHandler) packet(pkt scan.Packet) {
	if pkt.IsInterPacketTimeout() {
		mh.lg.Debug("inter-packet timeout detected during GPS configuration")
		for _, pp := range mh.packetProcs {
			pp.Idle(pkt.TRead)
		}
		return
	}
	data := pkt.Data
	if pkt.Format == nil {
		mh.invalid(data, pkt.ReadError)
		return
	}
	tag := pkt.Format.Tag()
	pp, ok := mh.packetProcs[tag]
	if !ok {
		mh.lg.Error("no processor registered for protocol", "protocol", tag)
		return
	}
	msgID, err := pp.ProcessPacket(data, pkt.TRead)
	if err != nil {
		mh.lg.Error("GPS packet cannot be parsed", "protocol", tag, "err", err)
		mh.bad.corruptMsgs++
	}
	// only count parseable messages with good checksum
	mh.msgCount[tag]++
	mh.msgIDs[tag][msgID] = true
}

func (mh *msgHandler) NativeMsg(tag gpsprot.Tag, msgID string, msg interface{}, tRead time.Time) error {
	mh.lg.Debug("received an unused message during configuration stage", "protocol", tag, "msgID", msgID, "msg", msg)
	if tag == nmea.Tag {
		nmeaLog(mh.lg, msg.(*nmea.Sentence))
	}
	return nil
}

func (mh *msgHandler) LeapSecond(ls *gpsprot.LeapSecondMsg, _ time.Time) {
	mh.leapSecond = ls
}

// Some number of unparseable bytes is normal.
// Log if we get more than this
const invalidBytesMaxExpected = 100

type SerialError interface {
	error
	FramingErrs() int
}

func (mh *msgHandler) invalid(data string, readErr error) {
	bad := &mh.bad
	n := bad.invalidBytes
	bad.invalidBytes += len(data)
	if bad.invalidBytes > invalidBytesMaxExpected && n <= invalidBytesMaxExpected {
		mh.lg.Debug("unexpectedly large number of unparseable bytes while starting to read GPS output")
	}
	if readErr != nil {
		if err, ok := readErr.(SerialError); ok && err.FramingErrs() > 0 {
			if bad.framingErrs == 0 {
				mh.lg.Info("framing errors reading GPS output during initialization")
			}
			bad.framingErrs += err.FramingErrs()
		} else {
			// Don't expect these
			mh.lg.Info("error reading GPS output during initialization", "err", readErr)
		}
	}
}

func (bc1 badCount) Sub(bc2 badCount) badCount {
	return badCount{
		invalidBytes: bc1.invalidBytes - bc2.invalidBytes,
		corruptMsgs:  bc1.corruptMsgs - bc2.corruptMsgs,
		framingErrs:  bc1.framingErrs - bc2.framingErrs,
	}
}

func nmeaLog(lg *slog.Logger, sen *nmea.Sentence) {
	if sen.Format == "TXT" && len(sen.Fields) >= 4 {
		// When we open an ACM device, the GPS receiver sends TXT messages with each line of the boot screen
		lg.Debug("received NMEA TXT message", "s", sen.Fields[3])
	}
}
