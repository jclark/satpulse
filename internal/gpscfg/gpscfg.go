package gpscfg

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/jclark/satpulse/internal/gpsio"
	"github.com/jclark/satpulse/internal/gpsmsg"
	"github.com/jclark/satpulse/internal/nmea"
	"github.com/jclark/satpulse/internal/scan"
	"github.com/jclark/satpulse/internal/ubx"
	ubxbin "github.com/jclark/satpulse/internal/ubx/bin"
	"golang.org/x/exp/maps"
)

type Result struct {
	Version    *ubx.Version
	ConfigMap  *gpsmsg.ConfigMap
	LeapSecond *gpsmsg.LeapSecond
}

type msgHandler struct {
	gpsmsg.DefaultHandler
	lg            *slog.Logger
	packetCh      <-chan scan.Packet
	ubxMsgCount   int
	nmeaMsgCount  int
	rtcmMsgCount  int
	bad           badCount
	nmeaSentences map[string]map[string]bool
	rtcmMsgs      map[uint16]bool
	ubxProt       *ubx.Protocol
	leapSecond    *gpsmsg.LeapSecond
}

type badCount struct {
	invalidBytes, corruptMsgs, framingErrs int
}

var _ ubx.ProtHandler = &msgHandler{}

func Configure(ctx context.Context, lg *slog.Logger, target *gpsmsg.ConfigMap, opts gpsmsg.ConfigOptions, packetCh <-chan scan.Packet, port gpsio.OutPort) (*Result, error) {
	mh := msgHandler{}
	mh.init(lg, packetCh)
	var err error
	if opts.Detect {
		err = mh.detect(ctx)
		if err != nil {
			return nil, err
		}
	}
	// After we have done detection, bad stuff is a cause for concern.
	badStart := mh.bad
	ubxOK, err := mh.probe(ctx, mh.ubxProt, port)
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
	var cm *gpsmsg.ConfigMap
	if ubxOK {
		cm, err = mh.configure(ctx, mh.ubxProt, target, opts, port)
		if err != nil {
			// XXX try to recover from this
			// provided we have some messages working, we should be OK
			return nil, err
		}
	} else {
		// XXX if ubxMsgCount > 0, then probably we cannot send to the GPS
		lg.Info("GPS does not respond to UBX messages; continuing hopefully")

	}
	return mh.finish(cm), nil
}

func RequiredConfig() *gpsmsg.ConfigMap {
	cm := &gpsmsg.ConfigMap{}
	cm.SetPPS()
	// Config is very slow on 8-th gen if NMEA is enabled
	gpsmsg.CfgNMEAEnabled.Set(cm, false)
	return cm
}

func RequiredOptions() gpsmsg.ConfigOptions {
	return gpsmsg.ConfigOptions{EnableLeapSecondMsg: true, EnableTimeMsg: true, Detect: true}
}

func (mh *msgHandler) init(lg *slog.Logger, packetCh <-chan scan.Packet) {
	mh.lg = lg
	mh.packetCh = packetCh
	mh.ubxProt = &ubx.Protocol{}
	mh.ubxProt.SetHandler(mh)
	mh.ubxProt.SetProtHandler(mh)
	mh.nmeaSentences = map[string]map[string]bool{}
}

func (mh *msgHandler) finish(cm *gpsmsg.ConfigMap) *Result {
	lg := mh.lg
	if cm != nil {
		lg.Info("GPS configuration", "cfg", cm)
	}
	if mh.leapSecond != nil {
		lsdStr := mh.leapSecond.Date().Format("2006-01-02")
		lg.Info("leap second information received from GPS", "date", lsdStr, "utcOffBefore", mh.leapSecond.UTCOffBefore, "utcOffAfter", mh.leapSecond.UTCOffAfter)
	}
	ver := mh.ubxProt.Version()
	if ver != nil {
		lg.Info("GPS version", "model", ver.Mod, "category", ver.ProductCategory(), "flash", ver.Flash,
			"sw", ver.SW, "hw", ver.HW, "prot", ver.Prot, "gnss", ver.GNSS, "ext", ver.Extensions)
	}
	lg.Info("finished GPS initialization",
		"nmeaSentences", maps.Keys(mh.nmeaSentences))
	return &Result{
		Version:    ver,
		ConfigMap:  cm,
		LeapSecond: mh.leapSecond,
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
		} else if mh.rtcmMsgCount > 0 {
			msg = "only RTCM messages detected from GPS"
		} else if mh.bad.invalidBytes+mh.bad.corruptMsgs == 0 {
			msg = "no output detected from GPS"
		} else if mh.bad.corruptMsgs > 0 {
			msg = "corrupted GPS output (multiple processes reading from serial port?)"
		} else {
			msg = "cannot parse GPS output"
		}
		lg.Debug("not receiving data from GPS correctly", "bad", mh.bad, "rtcmMsgCount", mh.rtcmMsgCount)
		return errors.New(msg)
	}
	lg.Info("detected a GPS")

	lg.Debug("received suitable output message from GPS", "isUBX", mh.ubxMsgCount > 0, "bad", mh.bad)
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

func (mh *msgHandler) probe(ctx context.Context, prot gpsmsg.Protocol, port gpsio.OutPort) (bool, error) {
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

func (mh *msgHandler) configure(ctx context.Context, prot gpsmsg.Protocol, target *gpsmsg.ConfigMap, opts gpsmsg.ConfigOptions, port gpsio.OutPort) (*gpsmsg.ConfigMap, error) {
	cfgtor, err := prot.Configure(target, opts)
	if err != nil {
		return nil, err
	}
	for {
		req, err := cfgtor.NextRequest()
		if err != nil {
			return nil, err
		}
		if req == nil {
			break
		}
		t := time.Now()
		pkt := req.Packet()
		_, err = port.Write(pkt)
		if err != nil {
			return nil, err
		}
		err = mh.waitAfterSend(ctx, prot, req, pkt, t, port)
		if err != nil {
			return nil, err
		}
	}
	return cfgtor.ConfigMap(), nil
}

const minWaitAfterSend = 10 * time.Millisecond

func (mh *msgHandler) waitAfterSend(ctx context.Context, prot gpsmsg.Protocol, req gpsmsg.ConfigRequest, pkt []byte, tSend time.Time, port gpsio.OutPort) error {
	ackable := req.Ackable()
	w := time.Millisecond * 1500
	reqID := req.ID()
	mh.lg.Debug("sent configuration message", "msgID", reqID, "len", len(pkt))
	if !ackable {
		w = max(port.TransmitTime(len(pkt)), minWaitAfterSend)
	}
	timerCh := time.After(w)
	for timerCh != nil {
		select {
		case packet, ok := <-mh.packetCh:
			if !ok {
				return mh.packetChClosed(ctx)
			}
			mh.packet(packet)
			if ackable {
				ack := prot.FindAck(pkt, tSend)
				if ack != nil {
					if !ack.OK {
						return fmt.Errorf("configuration message %s rejected", reqID)
					}
					req.Done()
					mh.lg.Debug("configuration accepted", "msgID", reqID, "delay", ack.TRead.Sub(tSend).String())
					return nil
				}
			}

		case <-timerCh:
			timerCh = nil
		}
	}
	if ackable {
		return fmt.Errorf("no ack received for configuration message %s", reqID)
	}
	return nil
}

func (mh *msgHandler) suitableMessageCount() int {
	return mh.nmeaMsgCount + mh.ubxMsgCount
}

func (mh *msgHandler) packet(f scan.Packet) {
	data := f.Data
	switch f.Kind {
	case scan.NMEA:
		mh.nmea(data)
	case scan.UBX:
		err := mh.ubxProt.ProcessPacket(data, f.TRead)
		if err != nil {
			mh.lg.Error("could not parse UBX message", "err", err)
			// UBX parsing can handle unknown message types, so it's something worse then that.
			mh.bad.corruptMsgs++
		} else {
			mh.ubxMsgCount++
		}
	case scan.RTCM:
		mh.rtcm(data)
	default:
		mh.invalid(data, f.ReadError)
	}
}

func (mh *msgHandler) LeapSecond(ls *gpsmsg.LeapSecond, _ time.Time) {
	mh.leapSecond = ls
}

func (mh *msgHandler) UBX(u ubxbin.Msg, _ time.Time) {
	mh.lg.Debug("received a UBX message during initialization", "id", u.ID().String(), "payload", u)
}

func (mh *msgHandler) nmea(data string) {
	lg := mh.lg
	msg, err := nmea.Parse(data)
	if err != nil {
		lg.Debug("received an NMEA message with invalid checksum during initialization")
		mh.bad.corruptMsgs++
		return
	}
	mh.nmeaMsgCount++
	talkerMap := mh.nmeaSentences[msg.SentenceFmt]
	if talkerMap == nil {
		talkerMap = map[string]bool{}
		mh.nmeaSentences[msg.SentenceFmt] = talkerMap
	}
	talkerMap[msg.TalkerID] = true
	nmeaLog(lg, msg)
}

func (mh *msgHandler) rtcm(data string) {
	lg := mh.lg
	_, ok, msgType := scan.RTCMMsg(data)
	if !ok {
		lg.Debug("received an RTCM message with invalid checksum during initialization")
		mh.bad.corruptMsgs++
		return
	}
	lg.Debug("received a RTCM message during initialization", "msgType", msgType)
	mh.rtcmMsgCount++
	mh.rtcmMsgs[msgType] = true
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

func nmeaLog(lg *slog.Logger, msg *nmea.Message) {
	if msg.SentenceFmt == "TXT" && len(msg.Fields) >= 4 {
		// When we open an ACM device, the GPS receiver sends TXT messages with each line of the boot screen
		lg.Debug("received NMEA TXT message", "s", msg.Fields[3])
	}
}
