package main

import (
	"context"
	"errors"
	"time"

	"github.com/jclark/gps2phc/scan"
	"github.com/jclark/gps2phc/serio"
	"github.com/jclark/gps2phc/ubx"
	"golang.org/x/exp/maps"
	"golang.org/x/exp/slog"
)

type gpsReceived struct {
	ubxMsgCount      int
	nmeaMsgCount     int
	invalidMsgCount  int
	invalidByteCount int
	nmeaSentences    map[string]map[string]bool
	protVer          ubx.ProtVer
	tmode2           *ubx.CfgTmode2
	tp5              *ubx.CfgTp5
	gnss             *ubx.CfgGNSS
	ack              map[ubx.MsgID]bool
}

func gpsInit(ctx context.Context, port *serio.Port) (frameCh chan scan.Frame, err error) {
	frameCh = port.StartRead(ctx)
	// must wait for writeRespCh before returning
	// so the called can close the Term without a data race
	configMsgs := [][]byte{
		ubx.Poll(ubx.MonVerID),
		ubx.Poll(ubx.CfgGNSSID),
		ubx.Poll(ubx.CfgTmode2ID),
		ubx.Poll(ubx.CfgTp5ID),
		ubx.Poll(ubx.TimSvinID),
		ubx.SetRate(ubx.NavTimeGPSID, 1),
		ubx.SetRate(ubx.TimTPID, 1),
	}
	writeRespCh := port.WriteAsync(ctx, configMsgs)
	timerCh := time.After(time.Second * 2)
	cancelCh := ctx.Done()
	lg := slog.FromContext(ctx)
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
		case <-cancelCh:
			cancelCh = nil
			if err != nil {
				err = ctx.Err()
			}
		case e := <-writeRespCh:
			writeRespCh = nil
			if e != nil && err == nil {
				err = e
			}
		case <-timerCh:
			timerCh = nil
		}
		if writeRespCh == nil {
			if err != nil {
				return
			}
			if timerCh == nil || frameCh == nil {
				break
			}
		}
	}
	if gr.ubxMsgCount+gr.nmeaMsgCount == 0 {
		if gr.invalidByteCount+gr.invalidMsgCount == 0 {
			err = errors.New("no output detected from GPS")
		} else if gr.invalidMsgCount > 0 {
			err = errors.New("invalid GPS output (multiple processes reading from serial port?)")
		} else {
			err = errors.New("unrecognized GPS receiver protocol")
		}
		return
	}
	gnssEnabled := []byte{}
	if gr.gnss != nil {
		for _, b := range gr.gnss.Blocks {
			if b.Enable != 0 {
				gnssEnabled = append(gnssEnabled, b.GNSSID)
			}
		}
	}
	lg.Debug("gpsInitDone",
		"nmeaSentences", maps.Keys(gr.nmeaSentences),
		"protVer", gr.protVer,
		"ack", gr.ack,
		"gnssEnabled", gnssEnabled)
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
	case *ubx.CfgTp5:
		gr.tp5 = data
	case *ubx.CfgGNSS:
		gr.gnss = data
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

func (gr *gpsReceived) invalid(data string, lg *slog.Logger) {
	gr.invalidByteCount += len(data)
}
