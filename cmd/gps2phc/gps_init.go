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
	ubxbin "github.com/jclark/gps2phc/internal/ubx/bin"
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
	tmode2           *ubxbin.CfgTmode2
	tmode3           *ubxbin.CfgTmode3
	tp5              *ubxbin.CfgTp5
	gnss             *ubxbin.CfgGNSS
	leapSecond       *ubx.LeapSecond
	version          *ubx.Version
	ack              map[ubxbin.MsgID]bool
}

func gpsInit(ctx context.Context, frameCh <-chan scan.Frame, port serio.OutPort) (err error) {
	// must wait for writeRespCh before returning
	// so the called can close the Term without a data race
	configMsgs := [][]byte{
		ubxbin.Poll(ubxbin.MonVerID),
		ubxbin.Poll(ubxbin.CfgGNSSID),
		ubxbin.Poll(ubxbin.CfgTmode2ID),
		ubxbin.Poll(ubxbin.CfgTmode3ID),
		ubxbin.Poll(ubxbin.CfgTp5ID),
		ubxbin.Poll(ubxbin.TimSvinID),
		ubxbin.Poll(ubxbin.NavSvinID),
		ubxbin.Poll(ubxbin.NavTimeLSID),
		ubxbin.SetRate(ubxbin.NavTimeGPSID, 1),
		ubxbin.SetRate(ubxbin.TimTPID, 1),
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
		lsdStr = gr.leapSecond.Date.Format("2006-01-02")
	}
	var tmode any = nil
	if gr.tmode2 != nil {
		tmode = gr.tmode2
	} else if gr.tmode3 != nil {
		tmode = gr.tmode3
	}
	if gr.version != nil {
		lg.Info("gpsVersion", "model", gr.version.Mod, "category", gr.version.FW.ProductCategory, "flash", gr.version.Flash,
			"sw", gr.version.SW, "hw", gr.version.HW, "prot", gr.version.Prot, "gnss", gr.version.GNSS, "ext", gr.version.Extensions)
	}
	lg.Info("gpsInitDone",
		"nmeaSentences", maps.Keys(gr.nmeaSentences),
		"ack", gr.ack,
		"gnssEnabled", gnssEnabled,
		"tmode", tmode,
		"leapSecDate", lsdStr)
	return
}

func (gr *gpsReceived) init() {
	gr.nmeaSentences = map[string]map[string]bool{}
	gr.ack = map[ubxbin.MsgID]bool{}
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
	um, err := ubx.Parse(data)
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
	ver := um.Version()
	if ver != nil {
		gr.version = ver
	}
	switch parsed := u.(type) {
	case *ubxbin.CfgTmode2:
		gr.tmode2 = parsed
	case *ubxbin.CfgTmode3:
		gr.tmode3 = parsed
	case *ubxbin.CfgTp5:
		gr.tp5 = parsed
	case *ubxbin.CfgGNSS:
		gr.gnss = parsed
	case *ubxbin.AckAck:
		gr.ack[parsed.MsgID] = true
	case *ubxbin.AckNak:
		gr.ack[parsed.MsgID] = false
	case *ubxbin.CfgMsg:
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
