package gpscfg

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/jclark/gps4ptp/internal/gpsmsg"
	"github.com/jclark/gps4ptp/internal/nmea"
	"github.com/jclark/gps4ptp/internal/scan"
	"github.com/jclark/gps4ptp/internal/serio"
	"github.com/jclark/gps4ptp/internal/ubx"
	ubxbin "github.com/jclark/gps4ptp/internal/ubx/bin"
	ubxcfg "github.com/jclark/gps4ptp/internal/ubx/cfg"
	"golang.org/x/exp/maps"
)

type InitData struct {
	Version  *ubx.Version     `json:"version,omitempty"`
	TimeMode *gpsmsg.TimeMode `json:"timeMode,omitempty"`
}

type msgHandler struct {
	gpsmsg.DefaultHandler
	lg            *slog.Logger
	ubxMsgCount   int
	nmeaMsgCount  int
	rtcmMsgCount  int
	bad           badCount
	nmeaSentences map[string]map[string]bool
	rtcmMsgs      map[uint16]bool
	ubxCfg        *ubx.Config
	leapSecond    *gpsmsg.LeapSecond
	ack           map[ubxbin.MsgID]bool
}

type badCount struct {
	invalidBytes, corruptMsgs, framingErrs int
}

var _ ubx.ProtHandler = &msgHandler{}

func Configure(ctx context.Context, lg *slog.Logger, packetCh <-chan scan.Packet, port serio.OutPort) (*InitData, error) {
	mh := msgHandler{}
	mh.init(lg)
	err := mh.detect(ctx, packetCh, port)
	if err != nil {
		return nil, err
	}
	return mh.configure(ctx, packetCh, port)
}

func (mh *msgHandler) detect(ctx context.Context, packetCh <-chan scan.Packet, port serio.OutPort) error {
	// Stage 1: Validate that we are receiving data correctly from a GPS.
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
	for packetCh != nil {
		select {
		case packet, ok := <-packetCh:
			if !ok {
				err := ctx.Err()
				if err != nil {
					return err
				}
				packetCh = nil
			} else {
				mh.packet(packet)
			}
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

func (mh *msgHandler) configure(ctx context.Context, packetCh <-chan scan.Packet, port serio.OutPort) (initData *InitData, err error) {
	// Stage 2: send some configuration messages and see what we get back
	badStart := mh.bad
	// must wait for writeRespCh before returning
	// so the called can close the Term without a data race
	configMsgs := [][]byte{
		ubxbin.Poll(ubxbin.MonVerID),
		ubxbin.Poll(ubxbin.CfgGNSSID),
		ubxbin.Poll(ubxbin.CfgRateID),
		ubxbin.Poll(ubxbin.CfgTmode2ID),
		ubxbin.Poll(ubxbin.CfgTmode3ID),
		ubxbin.Poll(ubxbin.CfgTp5ID),
		ubxbin.Poll(ubxbin.TimSvinID),
		ubxbin.Poll(ubxbin.NavSvinID),
		ubxbin.Poll(ubxbin.NavTimeLSID),
		ubxbin.SetRate(ubxbin.NavTimeGPSID, 1),
		ubxbin.SetRate(ubxbin.TimTPID, 1),
		tpTimegridGPS(),
	}
	writeRespCh := serio.WriteAsync(ctx, port, configMsgs)
	// We wait two seconds here for a response.
	timerCh := time.After(time.Millisecond * 2000)
	cancelCh := ctx.Done()
	for {
		select {
		case packet, ok := <-packetCh:
			if ok {
				mh.packet(packet)
			} else {
				packetCh = nil
			}
		// XXX This is not so useful right now, since cancelling will close the packetCh
		// But later we can use it to stop writing
		case <-cancelCh:
			cancelCh = nil
		case e := <-writeRespCh:
			writeRespCh = nil
			if e != nil && err == nil {
				err = e
			}
		case <-timerCh:
			timerCh = nil
		}
		if writeRespCh == nil {
			if err != nil || ctx.Err() != nil {
				return
			}
			if timerCh == nil || packetCh == nil {
				break
			}
		}
	}
	lg := mh.lg
	badNew := mh.bad.Sub(badStart)
	if mh.suitableMessageCount() < 2 || badNew.hasErrs() {
		var msg string
		if badNew.framingErrs > 0 {
			msg = "ongoing framing errors reading GPS output (hardware problems?)"
		} else if badNew.corruptMsgs > 0 {
			msg = "ongoing corrupted GPS output (multiple processes reading from serial port?)"
		} else {
			msg = "no regular output from GPS"
		}
		err = errors.New(msg)
		return
	}
	gnssEnabled := mh.ubxCfg.EnabledGNSS()
	var lsdStr string
	if mh.leapSecond != nil {
		lsdStr = mh.leapSecond.Date().Format("2006-01-02")
		lg.Info("leap second information received from GPS", "date", lsdStr, "utcOffBefore", mh.leapSecond.UTCOffBefore, "utcOffAfter", mh.leapSecond.UTCOffAfter)
	}
	if ver := mh.ubxCfg.Version(); ver != nil {
		lg.Info("GPS version", "model", ver.Mod, "category", ver.ProductCategory(), "flash", ver.Flash,
			"sw", ver.SW, "hw", ver.HW, "prot", ver.Prot, "gnss", ver.GNSS, "ext", ver.Extensions)
	}
	if period := mh.ubxCfg.SolutionPeriod(); period != 0 {
		lg.Debug("navigation/measurement rate", "period", period.String())
	}
	lg.Info("finished GPS initialization",
		"nmeaSentences", maps.Keys(mh.nmeaSentences),
		"ack", mh.ack,
		"gnssEnabled", gnssEnabled)
	initData = &InitData{
		TimeMode: mh.ubxCfg.TimeMode(),
		Version:  mh.ubxCfg.Version(),
	}
	return
}

func tpTimegridGPS() []byte {
	cfg := map[string]map[string]any{
		"TP": {
			"TIMEGRID_TP1": "GPS",
		},
	}
	u := ubxbin.CfgValset{
		CfgValsetFixed: ubxbin.CfgValsetFixed{
			Layers: ubxbin.CfgValsetLayerRAM,
		},
		CfgData: ubxcfg.GetSchema().MustMarshal(cfg),
	}
	bytes, err := ubxbin.Serialize(&u)
	if err != nil {
		panic(err)
	}
	return bytes
}

func (mh *msgHandler) init(lg *slog.Logger) {
	mh.lg = lg
	mh.nmeaSentences = map[string]map[string]bool{}
	mh.ack = map[ubxbin.MsgID]bool{}
}

func (mh *msgHandler) suitableMessageCount() int {
	return mh.nmeaMsgCount + mh.ubxMsgCount
}

func (mh *msgHandler) validMessageCount() int {
	return mh.suitableMessageCount() + mh.rtcmMsgCount
}

func (mh *msgHandler) packet(f scan.Packet) {
	data := f.Data
	switch f.Kind {
	case scan.NMEA:
		mh.nmea(data)
	case scan.UBX:
		err := ubx.ProcessPacketData(data, f.TRead, mh, mh)
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

func (mh *msgHandler) Version(ver *ubx.Version, _ time.Time) {
	mh.ubxCfg = ubx.NewConfig(ver)
}

func (mh *msgHandler) Ack(msgID ubxbin.MsgID, ok bool, _ time.Time) {
	mh.ack[msgID] = ok
}

func (mh *msgHandler) UBX(u ubxbin.Msg, _ time.Time) {
	if mh.ubxCfg.AddMsg(u) {
		return
	}
	lg := mh.lg
	switch parsed := u.(type) {
	case *ubxbin.CfgMsg:
		lg.Debug("got configured rate of UBX message", "id", parsed.MsgID, "rate", parsed.Rate)
	default:
		lg.Debug("received a UBX message during initialization", "id", u.ID().String(), "payload", u)
	}
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

func (bc badCount) hasErrs() bool {
	return bc.corruptMsgs != 0 || bc.framingErrs != 0
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
