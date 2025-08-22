package unc

import (
	"time"

	"github.com/jclark/satpulse/internal/gpsprot"
	"github.com/jclark/satpulse/internal/nov"
	"github.com/jclark/satpulse/internal/novmsg"
	"github.com/jclark/satpulse/internal/ptime"
	"github.com/jclark/satpulse/internal/uncmsg"
)

func timeRecTime(header uncmsg.MessageHeader, m *uncmsg.RecTime, tag gpsprot.Tag) (*gpsprot.TimeMsg, error) {
	if m.ClockStatus != novmsg.ClockStatusValid {
		return nil, nil
	}
	t := gpsprot.TimeMsg{
		Tag:         tag,
		NativeMsgID: "RECTIME",
	}
	gnss, toTAI := timeRefToTAI(header.TimeRef)
	t.GNSS = gnss
	if toTAI != nil && header.TimeStatus == uncmsg.TimeStatusFine {
		tow := time.Duration(header.MillisecondsOfWeek) * time.Millisecond
		t.TAITime = toTAI(int16(header.Week), tow)
	}

	return nov.TimeMsgSetUTC(&t, &m.Time)
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
