package main

import (
	"context"
	"errors"
	"time"

	"github.com/jclark/gps2phc/internal/logctx"
	"github.com/jclark/gps2phc/internal/nmea"
	"github.com/jclark/gps2phc/internal/scan"
	"github.com/jclark/gps2phc/internal/serio"
	"github.com/jclark/gps2phc/internal/ubx"
	"github.com/jclark/gps2phc/internal/ubxmsg"
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
	leapSecond       *ubxmsg.LeapSecond
	ack              map[ubx.MsgID]bool
}

func gpsInit(ctx context.Context, frameCh <-chan scan.Frame, port serio.OutPort) (err error) {
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
	writeRespCh := serio.WriteAsync(ctx, port, configMsgs)
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
	if gr.leapSecond != nil {
		lsdStr = gr.leapSecond.Date.Format("2006-01-02")
	}
	var tmode any = nil
	if gr.tmode2 != nil {
		tmode = gr.tmode2
	} else if gr.tmode3 != nil {
		tmode = gr.tmode3
	}
	lg.Info("gpsInitDone",
		"nmeaSentences", maps.Keys(gr.nmeaSentences),
		"protVer", gr.protVer,
		"ack", gr.ack,
		"gnssEnabled", gnssEnabled,
		"tmode", tmode,
		"leapSecDate", lsdStr)
	return
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
	um, err := ubxmsg.Parse(data)
	if err != nil {
		lg.Error("ubxParseError", err)
		gr.invalidMsgCount++
		return
	}
	gr.ubxMsgCount++
	u := um.UBX()
	ls := um.LeapSecond()
	if ls != nil {
		gr.leapSecond = ls
	}
	switch parsed := u.(type) {
	case *ubx.MonVer:
		gr.protVer = parsed.ProtVer()
		lg.Info("gpsVersion", "sw", ubx.Latin1ZToString(parsed.SwVersion[:]), "hw", ubx.Latin1ZToString(parsed.HwVersion[:]), "protVer", gr.protVer)
	case *ubx.CfgTmode2:
		gr.tmode2 = parsed
	case *ubx.CfgTmode3:
		gr.tmode3 = parsed
	case *ubx.CfgTp5:
		gr.tp5 = parsed
	case *ubx.CfgGNSS:
		gr.gnss = parsed
	case *ubx.AckAck:
		gr.ack[parsed.MsgID] = true
	case *ubx.AckNak:
		gr.ack[parsed.MsgID] = false
	case *ubx.CfgMsg:
		lg.Debug("ubxRate", "id", parsed.MsgID, "rate", parsed.Rate)
	default:
		lg.Debug("ubx", "id", u.ID().String(), "payload", u)
	}
}

func (gr *gpsReceived) nmea(data string, lg *slog.Logger) {
	m, err := nmea.Parse(data)
	if err != nil {
		gr.invalidMsgCount++
		return
	}
	gr.nmeaMsgCount++
	fields := m.Fields()
	talkerMap := gr.nmeaSentences[fields.SentenceFmt]
	if talkerMap == nil {
		talkerMap = map[string]bool{}
		gr.nmeaSentences[fields.SentenceFmt] = talkerMap
	}
	talkerMap[fields.TalkerID] = true
	nmeaLog(lg, m)
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
