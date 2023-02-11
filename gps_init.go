package main

import (
	"context"
	"errors"
	"time"

	"github.com/jclark/gps2phc/logctx"
	"github.com/jclark/gps2phc/ptime"
	"github.com/jclark/gps2phc/scan"
	"github.com/jclark/gps2phc/serio"
	"github.com/jclark/gps2phc/ubx"
	"golang.org/x/exp/maps"
	"golang.org/x/exp/slog"
)

type gpsReceived struct {
	ubxMsgCount      int
	nmeaMsgCount     int
	rtcmMsgCount     int
	invalidMsgCount  int
	invalidByteCount int
	nmeaSentences    map[string]map[string]bool
	rtcmMsgs         map[uint16]bool
	protVer          ubx.ProtVer
	tmode2           *ubx.CfgTmode2
	tmode3           *ubx.CfgTmode3
	tp5              *ubx.CfgTp5
	gnss             *ubx.CfgGNSS
	timeLS           *ubx.NavTimeLS
	ack              map[ubx.MsgID]bool
}

// If returned frameCh is not nil, then reading goroutine is still running.
func gpsInit(ctx context.Context, port *serio.Port) (frameCh <-chan scan.Frame, err error) {
	frameCh = port.StartRead(ctx)
	// must wait for writeRespCh before returning
	// so the called can close the Term without a data race
	configMsgs := [][]byte{
		ubx.Poll(ubx.MonVerID),
		ubx.Poll(ubx.CfgGNSSID),
		ubx.Poll(ubx.CfgTmode2ID),
		ubx.Poll(ubx.CfgTmode3ID),
		ubx.Poll(ubx.CfgTp5ID),
		ubx.Poll(ubx.TimSvinID),
		ubx.Poll(ubx.NavSvinID),
		ubx.Poll(ubx.NavTimeLSID),
		ubx.SetRate(ubx.NavTimeGPSID, 1),
		ubx.SetRate(ubx.TimTPID, 1),
	}
	writeRespCh := port.WriteAsync(ctx, configMsgs)
	timerCh := time.After(time.Second * 2)
	cancelCh := ctx.Done()
	lg := logctx.FromContext(ctx)
	gr := gpsReceived{}
	gr.init()
	for {
		select {
		case frame, ok := <-frameCh:
			if ok {
				gr.frame(frame.Kind, string(frame.Data), lg)
			} else {
				frameCh = nil
			}
		// XXX This is not so useful right now, since cancelling will close the frameCh
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
			if timerCh == nil || frameCh == nil {
				break
			}
		}
	}
	if gr.ubxMsgCount+gr.nmeaMsgCount+gr.rtcmMsgCount == 0 {
		if gr.invalidByteCount+gr.invalidMsgCount == 0 {
			err = errors.New("no output detected from GPS")
		} else if gr.invalidMsgCount > 0 {
			err = errors.New("invalid GPS output (multiple processes reading from serial port?)")
		} else {
			err = errors.New("unrecognized GPS receiver protocol")
		}
		return
	}
	gnssEnabled := []ubx.GNSSID{}
	if gr.gnss != nil {
		for _, b := range gr.gnss.Blocks {
			if b.Enable != 0 {
				gnssEnabled = append(gnssEnabled, b.GNSSID)
			}
		}
	}
	var lsdStr string
	if gr.timeLS != nil {
		lsd := leapSecondDate(gr.timeLS)
		if !lsd.IsZero() {
			lsdStr = lsd.Format("2006-01-02")
		}
	}
	var tmode any = nil
	if gr.tmode2 != nil {
		tmode = gr.tmode2
	} else if gr.tmode3 != nil {
		tmode = gr.tmode3
	}
	lg.Debug("gpsInitDone",
		"nmeaSentences", maps.Keys(gr.nmeaSentences),
		"protVer", gr.protVer,
		"ack", gr.ack,
		"gnssEnabled", gnssEnabled,
		"tmode", tmode,
		"leapSecDate", lsdStr)
	return
}

func leapSecondDate(tls *ubx.NavTimeLS) time.Time {
	z := time.Time{}
	if (tls.Valid & ubx.NavTimeLSValidTimeToLSEvent) == 0 {
		return z
	}
	wd := tls.DateOfLSGPSDN
	switch tls.SrcOfLSChange {
	case ubx.NavTimeLSSrcOfLSChangeBeiDou:
		// BeiDou DN is 0-based
	case ubx.NavTimeLSSrcOfLSChangeGPS, ubx.NavTimeLSSrcOfLSChangeGalileo:
		// GPS and Galileo DN is 1-based
		wd--
	default:
		// No info about meaning of DN for other cases cases
		return z
	}
	t := ptime.GPSDate(tls.DateOfLSGPSWN, time.Weekday(wd))
	if isLastDayOfQuarter(t) {
		return t
	}
	if tls.LSChange == 0 {
		// This is a past change.
		// GPS transmits only the bottom 8-bits of the week number of the leap second
		// So a past leap second can be off by a multiple of 256 weeks.
		for i := 1; i <= 2; i++ {
			t = t.AddDate(0, 0, -7*0x100)
			if isLastDayOfQuarter(t) {
				return t
			}
		}
	}
	return z
}

func isLastDayOfQuarter(t time.Time) bool {
	return t.AddDate(0, 0, 1).Day() == 1 && t.Month()%3 == 0
}

func (gr *gpsReceived) init() {
	gr.nmeaSentences = map[string]map[string]bool{}
	gr.ack = map[ubx.MsgID]bool{}
}

func (gr *gpsReceived) frame(kind scan.FrameKind, data string, lg *slog.Logger) {
	switch kind {
	case scan.NMEA:
		gr.nmea(data, lg)
	case scan.UBX:
		gr.ubx(data, lg)
	case scan.RTCM:
		gr.rtcm(data, lg)
	default:
		gr.invalid(data, lg)
	}
}

func (gr *gpsReceived) ubx(data string, lg *slog.Logger) {
	u, err := ubx.ParseMsg(data)
	if err != nil {
		lg.Error("ubxParseError", err)
		gr.invalidMsgCount++
		return
	}
	gr.ubxMsgCount++
	switch data := u.(type) {
	case *ubx.MonVer:
		gr.protVer = data.ProtVer()
		lg.Info("gpsVersion", "sw", ubx.Latin1ZToString(data.SwVersion[:]), "hw", ubx.Latin1ZToString(data.HwVersion[:]), "protVer", gr.protVer)
	case *ubx.CfgTmode2:
		gr.tmode2 = data
	case *ubx.CfgTmode3:
		gr.tmode3 = data
	case *ubx.CfgTp5:
		gr.tp5 = data
	case *ubx.CfgGNSS:
		gr.gnss = data
	case *ubx.NavTimeLS:
		gr.timeLS = data
	case *ubx.AckAck:
		gr.ack[data.MsgID] = true
	case *ubx.AckNak:
		gr.ack[data.MsgID] = false
	case *ubx.CfgMsg:
		lg.Debug("ubxRate", "id", data.MsgID, "rate", data.Rate)
	default:
		lg.Debug("ubx", "id", u.ID().String(), "payload", u)
	}
}

func (gr *gpsReceived) nmea(data string, lg *slog.Logger) {
	fields := scan.NMEASplit(data)
	if !fields.ChecksumOK {
		gr.invalidMsgCount++
		return
	}
	gr.nmeaMsgCount++
	talkerMap := gr.nmeaSentences[fields.SentenceFmt]
	if talkerMap == nil {
		talkerMap = map[string]bool{}
		gr.nmeaSentences[fields.SentenceFmt] = talkerMap
	}
	talkerMap[fields.TalkerID] = true
	nmeaLog(lg, data)
}

func (gr *gpsReceived) rtcm(data string, lg *slog.Logger) {
	_, ok, msgType := scan.RTCMMsg(data)
	if !ok {
		gr.invalidMsgCount++
		return
	}
	gr.rtcmMsgCount++
	gr.rtcmMsgs[msgType] = true
}

func (gr *gpsReceived) invalid(data string, lg *slog.Logger) {
	gr.invalidByteCount += len(data)
}
