package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jclark/gps2phc/scan"
	"github.com/jclark/gps2phc/serio"
	"github.com/jclark/gps2phc/ubx"
	"golang.org/x/exp/maps"
	"golang.org/x/exp/slog"
)

func gpsInit(ctx context.Context, port *serio.Port) (frameCh chan scan.Frame, err error) {
	frameCh = port.ReadStart(ctx)
	// must wait for writeRespCh before returning
	// so the called can close the Term without a data race
	configMsgs := [][]byte{
		ubx.Poll[ubx.MonVer](),
		ubx.Poll[ubx.CfgTmode2](),
		ubx.Poll[ubx.CfgTp5](),
		ubx.Poll[ubx.TimSvin](),
		ubx.SetRate[ubx.NavTimeGPS](1),
		ubx.SetRate[ubx.TimTP](1),
	}
	writeRespCh := port.WriteAsync(ctx, configMsgs)
	timerCh := time.After(time.Second * 2)
	cancelCh := ctx.Done()
	nmeaMsgs := []string{}
	ubxMsgs := []string{}
	invalidByteCount := 0
	for {
		select {
		case frame, ok := <-frameCh:
			if ok {
				switch frame.Kind {
				case scan.NMEA:
					nmeaMsgs = append(nmeaMsgs, string(frame.Data))
				case scan.UBX:
					ubxMsgs = append(ubxMsgs, string(frame.Data))
				case scan.Invalid:
					invalidByteCount += len(frame.Data)
				}
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
	if len(ubxMsgs) == 0 && len(nmeaMsgs) == 0 {
		if invalidByteCount == 0 {
			err = errors.New("new output detected from GPS")
		} else {
			err = errors.New("could not understand GPS output")
		}
		return
	}
	lg := slog.FromContext(ctx)
	for _, msg := range ubxMsgs {
		u, err := ubx.ParseMsg(msg)
		if err != nil {
			lg.Error("ubxParseError", err)
		} else if u != nil {
			switch data := u.(type) {
			case *ubx.MonVer:
				major, minor := data.ProtVer()
				protVer := "?"
				if major >= 0 {
					protVer = fmt.Sprintf("%d.%02d", major, minor)
				}
				lg.Info("gpsVersion", "sw", ubx.Latin1ZToString(data.SwVersion[:]), "hw", ubx.Latin1ZToString(data.HwVersion[:]), "protver", protVer)
			default:
				lg.Debug("ubx", "type", u.ID().String(), "payload", u)
			}
		}
	}
	sentenceMap := map[string]map[string]bool{}
	for _, msg := range nmeaMsgs {
		fields := scan.NMEASplit(msg)
		talkerMap := sentenceMap[fields.SentenceFmt]
		if talkerMap == nil {
			talkerMap = map[string]bool{}
			sentenceMap[fields.SentenceFmt] = talkerMap
		}
		talkerMap[fields.TalkerID] = true
		nmeaLog(lg, msg)
	}
	sentences := maps.Keys(sentenceMap)
	lg.Debug("gpsInitDone", "nmeaSentences", sentences)
	return
}
