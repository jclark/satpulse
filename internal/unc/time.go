package unc

import (
	"time"

	"github.com/jclark/satpulse/internal/gpsprot"
	"github.com/jclark/satpulse/internal/nov"
	"github.com/jclark/satpulse/internal/novmsg"
	"github.com/jclark/satpulse/internal/ptime"
	"github.com/jclark/satpulse/internal/uncmsg"
)

func timeMsgFromRecTime(hdr *uncmsg.MsgHdr, recTime *uncmsg.RecTime, tag gpsprot.Tag) (*gpsprot.TimeMsg, error) {
	if recTime.ClockStatus != novmsg.ClockStatusValid {
		return nil, nil
	}
	t := gpsprot.TimeMsg{
		Tag:         tag,
		NativeMsgID: "RECTIME",
	}
	gnss, toTAI := timeRefToTAI(hdr.TimeRef)
	t.GNSS = gnss
	if toTAI != nil && hdr.TimeStatus == uncmsg.TimeStatusFine {
		tow := time.Duration(hdr.MillisecondsOfWeek) * time.Millisecond
		t.TAITime = toTAI(int16(hdr.Week), tow)
	}

	return nov.TimeMsgSetUTC(&t, &recTime.Time)
}

func timeRefToTAI(ref uncmsg.TimeRef) (gpsprot.GNSS, func(int16, time.Duration) ptime.Time) {
	switch ref {
	case uncmsg.TimeRefGPS:
		return gpsprot.GPS, ptime.GPS
	case uncmsg.TimeRefBDS:
		return gpsprot.BDS, ptime.BeiDou
	}
	return 0, nil
}
