package gpscfg

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/jclark/gps4ptp/internal/gpsmsg"
	"github.com/jclark/gps4ptp/internal/nmea"
	"github.com/jclark/gps4ptp/internal/scan"
	"github.com/jclark/gps4ptp/internal/serio"
	"github.com/jclark/gps4ptp/internal/ubx"
	ubxbin "github.com/jclark/gps4ptp/internal/ubx/bin"
	"golang.org/x/exp/maps"
)

type InitData struct {
	Version  *ubx.Version     `json:"version,omitempty"`
	TimeMode *gpsmsg.TimeMode `json:"timeMode,omitempty"`
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
	ubxCfg        *ubx.Config
	leapSecond    *gpsmsg.LeapSecond
}

type badCount struct {
	invalidBytes, corruptMsgs, framingErrs int
}

var _ ubx.ProtHandler = &msgHandler{}

func Configure(ctx context.Context, lg *slog.Logger, packetCh <-chan scan.Packet, port serio.OutPort) (*InitData, error) {
	mh := msgHandler{}
	mh.init(lg, packetCh)
	err := mh.detect(ctx)
	if err != nil {
		return nil, err
	}
	// After we have done detection, bad stuff is a cause for concern.
	badStart := mh.bad
	ubxOK, err := mh.ubxProbe(ctx, port)
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
	if !ubxOK {
		// XXX if ubxMsgCount > 0, then probably we cannot send to the GPS
		lg.Info("GPS does not respond to UBX messages; continuing hopefully")
	} else {
		err = mh.ubxConfigure(ctx, port)
		if err != nil {
			// XXX try to recover from this
			// provided we have some messages working, we should be OK
			return nil, err
		}
		mh.logUbxConfig()
	}
	initData := &InitData{
		TimeMode: mh.ubxCfg.TimeMode(),
		Version:  mh.ubxProt.Version(),
	}
	return initData, nil
}

func (mh *msgHandler) init(lg *slog.Logger, packetCh <-chan scan.Packet) {
	mh.lg = lg
	mh.packetCh = packetCh
	mh.ubxProt = &ubx.Protocol{}
	mh.ubxCfg = &ubx.Config{}
	mh.nmeaSentences = map[string]map[string]bool{}
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

func (mh *msgHandler) ubxProbe(ctx context.Context, port serio.OutPort) (bool, error) {
	msg := mh.ubxProt.PollVersion()
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
			if mh.ubxProt.Version() != nil {
				return true, nil
			}
		case <-timerCh:
			timerCh = nil
		}
	}
	return false, nil
}

func (mh *msgHandler) ubxConfigure(ctx context.Context, port serio.OutPort) error {
	packets := append(mh.ubxProt.GetterMsgs(),
		mh.ubxProt.PollSurveyIn(),
		mh.ubxProt.PollLeapSecond(),
		ubxbin.SetRate(ubxbin.NavTimeGPSID, 1),
		ubxbin.SetRate(ubxbin.TimTPID, 1))
	for _, pkt := range packets {
		t := time.Now()
		_, err := port.Write(pkt)
		if err != nil {
			return err
		}
		err = mh.waitAfterSend(ctx, pkt, t, port)
		if err != nil {
			return err
		}
	}
	return nil
}

const minWaitAfterSend = 10 * time.Millisecond

func (mh *msgHandler) waitAfterSend(ctx context.Context, pkt []byte, tSend time.Time, port serio.OutPort) error {
	ackable := ubx.PacketAckable(pkt)
	w := time.Millisecond * 1500
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
				ack := mh.ubxProt.FindAck(pkt, tSend)
				if ack != nil {
					msgID := ubx.PacketMsgID(pkt)
					if !ack.OK {
						return fmt.Errorf("configuration message %s rejected", msgID)
					}
					mh.lg.Debug("configuration accepted", "msgID", msgID, "delay", ack.TRead.Sub(tSend).String())
					return nil
				}
			}

		case <-timerCh:
			timerCh = nil
		}
	}
	if ackable {
		return fmt.Errorf("no ack received for configuration message %s", ubx.PacketMsgID(pkt))
	}
	return nil
}

func (mh *msgHandler) logUbxConfig() {
	lg := mh.lg
	gnssEnabled := mh.ubxCfg.EnabledGNSS()
	var lsdStr string
	if mh.leapSecond != nil {
		lsdStr = mh.leapSecond.Date().Format("2006-01-02")
		lg.Info("leap second information received from GPS", "date", lsdStr, "utcOffBefore", mh.leapSecond.UTCOffBefore, "utcOffAfter", mh.leapSecond.UTCOffAfter)
	}
	if ver := mh.ubxProt.Version(); ver != nil {
		lg.Info("GPS version", "model", ver.Mod, "category", ver.ProductCategory(), "flash", ver.Flash,
			"sw", ver.SW, "hw", ver.HW, "prot", ver.Prot, "gnss", ver.GNSS, "ext", ver.Extensions)
	}
	if period := mh.ubxCfg.SolutionPeriod(); period != 0 {
		lg.Debug("navigation/measurement rate", "period", period.String())
	}
	lg.Info("finished GPS initialization",
		"nmeaSentences", maps.Keys(mh.nmeaSentences),
		"gnssEnabled", gnssEnabled)
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
		err := mh.ubxProt.ProcessPacket(data, f.TRead, mh.ubxCfg, mh, mh)
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
