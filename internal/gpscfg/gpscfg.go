package gpscfg

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/jclark/gps4ptp/internal/gpsmsg"
	"github.com/jclark/gps4ptp/internal/logctx"
	"github.com/jclark/gps4ptp/internal/nmea"
	"github.com/jclark/gps4ptp/internal/scan"
	"github.com/jclark/gps4ptp/internal/serio"
	"github.com/jclark/gps4ptp/internal/ubx"
	ubxbin "github.com/jclark/gps4ptp/internal/ubx/bin"
	ubxcfg "github.com/jclark/gps4ptp/internal/ubx/cfg"
	"golang.org/x/exp/maps"
)

type GPSInitData struct {
	Version  *ubx.Version     `json:"version,omitempty"`
	TimeMode *gpsmsg.TimeMode `json:"timeMode,omitempty"`
}

// XXX this needs to be renamed
type gpsReceived struct {
	gpsmsg.DefaultHandler
	lg               *slog.Logger
	ubxMsgCount      int
	nmeaMsgCount     int
	rtcmMsgCount     int
	invalidMsgCount  int
	invalidByteCount int
	framingErrors    bool
	nmeaSentences    map[string]map[string]bool
	rtcmMsgs         map[uint16]bool
	tp5              *ubxbin.CfgTp5
	gnss             *ubxbin.CfgGNSS
	rate             *ubxbin.CfgRate
	leapSecond       *gpsmsg.LeapSecond
	timeMode         *gpsmsg.TimeMode
	version          *ubx.Version
	ack              map[ubxbin.MsgID]bool
}

func GpsInit(ctx context.Context, packetCh <-chan scan.Packet, port serio.OutPort) (initData *GPSInitData, err error) {
	lg := logctx.FromContext(ctx)
	gr := gpsReceived{}
	gr.init(lg)
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
				if ctx.Err() != nil {
					return
				}
				packetCh = nil
			} else {
				gr.packet(packet)
			}
		case <-timer1Ch:
			timer1Ch = nil
		case <-timer2Ch:
			timer2Ch = nil
		}
		if gr.suitableMessageCount() > 0 || timer2Ch == nil || (timer1Ch == nil && !gr.framingErrors) {
			break
		}
	}
	if gr.suitableMessageCount() == 0 {
		var msg string
		if gr.framingErrors {
			msg = "framing errors reading GPS output (wrong speed?)"
		} else if gr.rtcmMsgCount > 0 {
			msg = "only RTCM messages detected from GPS"
		} else if gr.invalidByteCount+gr.invalidMsgCount == 0 {
			msg = "no output detected from GPS"
		} else if gr.invalidMsgCount > 0 {
			msg = "corrupted GPS output (multiple processes reading from serial port?)"
		} else {
			msg = "cannot parse GPS output"
		}
		lg.Debug("not receiving data from GPS correctly",
			"framingErrors", gr.framingErrors,
			"initialInvalidByteCount", gr.invalidByteCount,
			"initialInvalidMsgCount", gr.invalidMsgCount,
			"rtcmMsgCount", gr.rtcmMsgCount)
		err = errors.New(msg)
		return
	}
	lg.Info("detected a GPS")
	lg.Debug("received suitable output message from GPS",
		"isUBX", gr.ubxMsgCount > 0,
		"framingErrors", gr.framingErrors,
		"initialInvalidByteCount", gr.invalidByteCount,
		"initialInvalidMsgCount", gr.invalidMsgCount)
	// Stage 2: send some configuration messages and see what we get back
	gr.invalidMsgCount = 0
	gr.invalidByteCount = 0
	gr.framingErrors = false
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
				gr.packet(packet)
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
	if gr.suitableMessageCount() < 2 || gr.invalidMsgCount > 0 {
		if gr.framingErrors {
			err = errors.New("ongoing framing errors reading GPS output (hardware problems?)")
		} else if gr.invalidMsgCount > 0 {
			err = errors.New("ongoing corrupted GPS output (multiple processes reading from serial port?)")
		} else {
			err = errors.New("no regular output from GPS")
		}
		return
	}
	gnssEnabled := []ubxbin.GNSSID{}
	if gr.gnss != nil {
		for _, b := range gr.gnss.Blocks {
			if b.Enable != 0 {
				gnssEnabled = append(gnssEnabled, b.GNSSID)
			}
		}
	}
	var lsdStr string
	if gr.leapSecond != nil {
		lsdStr = gr.leapSecond.Date().Format("2006-01-02")
		lg.Info("leap second information received from GPS", "date", lsdStr, "utcOffBefore", gr.leapSecond.UTCOffBefore, "utcOffAfter", gr.leapSecond.UTCOffAfter)
	}
	if gr.version != nil {
		lg.Info("GPS version", "model", gr.version.Mod, "category", gr.version.ProductCategory(), "flash", gr.version.Flash,
			"sw", gr.version.SW, "hw", gr.version.HW, "prot", gr.version.Prot, "gnss", gr.version.GNSS, "ext", gr.version.Extensions)
	}
	if gr.rate != nil {
		lg.Debug("navigation/measurement rate", "measRate", gr.rate.MeasRate, "navRate", gr.rate.NavRate, "timeRef", gr.rate.TimeRef)
	}
	lg.Info("finished GPS initialization",
		"nmeaSentences", maps.Keys(gr.nmeaSentences),
		"ack", gr.ack,
		"gnssEnabled", gnssEnabled)
	initData = &GPSInitData{
		TimeMode: gr.timeMode,
		Version:  gr.version,
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

func (gr *gpsReceived) init(lg *slog.Logger) {
	gr.lg = lg
	gr.nmeaSentences = map[string]map[string]bool{}
	gr.ack = map[ubxbin.MsgID]bool{}
}

func (gr *gpsReceived) suitableMessageCount() int {
	return gr.nmeaMsgCount + gr.ubxMsgCount
}

func (gr *gpsReceived) validMessageCount() int {
	return gr.suitableMessageCount() + gr.rtcmMsgCount
}

func (gr *gpsReceived) packet(f scan.Packet) {
	data := f.Data
	switch f.Kind {
	case scan.NMEA:
		gr.nmea(data)
	case scan.UBX:
		err := ubx.ProcessPacketData(data, f.TRead, gr, gr)
		if err != nil {
			gr.lg.Error("could not parse UBX message", "err", err)
			// UBX parsing can handle unknown message types, so it's something worse then that.
			gr.invalidMsgCount++
		} else {
			gr.ubxMsgCount++
		}
	case scan.RTCM:
		gr.rtcm(data)
	default:
		gr.invalid(data, f.ReadError)
	}
}

func (gr *gpsReceived) LeapSecond(ls *gpsmsg.LeapSecond, _ time.Time) {
	gr.leapSecond = ls
}

func (gr *gpsReceived) Version(ver *ubx.Version, _ time.Time) {
	gr.version = ver
}

func (gr *gpsReceived) TimeMode(tm *gpsmsg.TimeMode, _ time.Time) {
	gr.timeMode = tm
}

func (gr *gpsReceived) UBX(u ubxbin.Msg, _ time.Time) {
	lg := gr.lg
	switch parsed := u.(type) {
	case *ubxbin.CfgTp5:
		gr.tp5 = parsed
	case *ubxbin.CfgGNSS:
		gr.gnss = parsed
	case *ubxbin.CfgRate:
		gr.rate = parsed
	case *ubxbin.AckAck:
		gr.ack[parsed.MsgID] = true
	case *ubxbin.AckNak:
		gr.ack[parsed.MsgID] = false
	case *ubxbin.CfgMsg:
		lg.Debug("got configured rate of UBX message", "id", parsed.MsgID, "rate", parsed.Rate)
	default:
		lg.Debug("received a UBX message during initialization", "id", u.ID().String(), "payload", u)
	}
}

func (gr *gpsReceived) nmea(data string) {
	lg := gr.lg
	msg, err := nmea.Parse(data)
	if err != nil {
		lg.Debug("received an NMEA message with invalid checksum during initialization")
		gr.invalidMsgCount++
		return
	}
	gr.nmeaMsgCount++
	lg.Debug("received an NMEA message during initialization", "sentence", msg.TalkerID+msg.SentenceFmt)
	talkerMap := gr.nmeaSentences[msg.SentenceFmt]
	if talkerMap == nil {
		talkerMap = map[string]bool{}
		gr.nmeaSentences[msg.SentenceFmt] = talkerMap
	}
	talkerMap[msg.TalkerID] = true
	nmeaLog(lg, msg)
}

func (gr *gpsReceived) rtcm(data string) {
	lg := gr.lg
	_, ok, msgType := scan.RTCMMsg(data)
	if !ok {
		lg.Debug("received an RTCM message with invalid checksum during initialization")
		gr.invalidMsgCount++
		return
	}
	lg.Debug("received a RTCM message during initialization", "msgType", msgType)
	gr.rtcmMsgCount++
	gr.rtcmMsgs[msgType] = true
}

// Some number of unparseable bytes is normal.
// Log if we get more than this
const invalidByteCountMaxExpected = 100

type SerialError interface {
	error
	Frame() bool
}

func (gr *gpsReceived) invalid(data string, readErr error) {
	n := gr.invalidByteCount
	gr.invalidByteCount += len(data)
	if gr.invalidByteCount > invalidByteCountMaxExpected && n <= invalidByteCountMaxExpected {
		gr.lg.Debug("unexpectedly large number of unparseable bytes while starting to read GPS output")
	}
	if readErr != nil {
		if err, ok := readErr.(SerialError); ok && err.Frame() {
			if !gr.framingErrors {
				gr.lg.Info("framing errors reading GPS output during initialization")
			}
			gr.framingErrors = true
		} else {
			// Don't expect these
			gr.lg.Info("error reading GPS output during initialization", "err", readErr)
		}
	}
}

func nmeaLog(lg *slog.Logger, msg *nmea.Message) {
	if msg.SentenceFmt == "TXT" && len(msg.Fields) >= 4 {
		// When we open an ACM device, the GPS receiver sends TXT messages with each line of the boot screen
		lg.Debug("received NMEA TXT message", "s", msg.Fields[3])
	}
}
