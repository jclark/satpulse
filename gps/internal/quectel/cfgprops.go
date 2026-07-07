package quectel

import (
	"fmt"
	"time"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/qtmmsg"
)

// convertToProps populates props from the as-found tuples. A nil
// tuple contributes nothing: its properties are absent.
func (f *asFound) convertToProps(props *gpsprot.ConfigProps) {
	if f.uart != nil {
		props.SetBaudRate(f.uart.BaudRate)
		props.SetPort(fmt.Sprintf("UART%d", f.uart.Index))
	}
	if f.eleThd != nil {
		props.SetMinElevation(gpsprot.DegreesFromFloat(float64(f.eleThd.Ele)))
	}
	if f.rsid != nil {
		props.SetRTCMBaseID(f.rsid.ID)
	}
	if f.pps2 != nil {
		setTimePulse(props, f.pps2.Enable, f.pps2.Duration, f.pps2.Mode, f.pps2.Polarity,
			time.Duration(f.pps2.Period)*time.Millisecond)
	} else if f.pps != nil {
		setTimePulse(props, f.pps.Enable, f.pps.Duration, f.pps.Mode, f.pps.Polarity, time.Second)
	}
}

// setTimePulse converts a PPS/PPS2 tuple. The pulse is always aligned
// to the GNSS second - the protocol has no alignment knob - so
// AlignToGNSS is reported as the constant true. A disabled tuple
// reads back truncated (Duration through Userdelay unreported), so
// only the zero width and the alignment constant are knowable then.
func setTimePulse(props *gpsprot.ConfigProps, enable uint8, durMs uint16, mode, polarity uint8, period time.Duration) {
	if enable == 0 {
		props.SetTimePulseWidth(0)
		props.SetTimePulseAlignToGNSS(true)
		return
	}
	props.SetTimePulse(gpsprot.TimePulse{
		Width:          time.Duration(durMs) * time.Millisecond,
		Period:         period,
		AlignToGNSS:    true,
		OnlyWhenLocked: mode == qtmmsg.PPSModeFixOnly,
		PolarityRising: polarity == 1,
	})
}
