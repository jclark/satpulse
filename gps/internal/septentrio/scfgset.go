package septentrio

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jclark/satpulse/gps/gpsprot"
)

// Set requests realize the target's properties. Each set's ack echoes the
// achieved state line, which onReply parses back into nativeProps with the
// same parser the query used: the ack is the readback. A refusal leaves the
// receiver configuration unchanged (verified), so the queried value stands.
// The receiver refuses out-of-range values rather than clamping, so range
// clamping happens here, before the wire.

// gnssTimeScale is the reverse of timeScaleGNSS; other GNSS have no
// TimeScale analogue and leave the argument unset.
var gnssTimeScale = map[gpsprot.GNSS]string{
	gpsprot.GPS: "GPS",
	gpsprot.GAL: "Galileo",
	gpsprot.BDS: "BeiDou",
	gpsprot.GLO: "GLONASS",
}

// generateSetReqs generates the set requests for the scalar properties.
func (c *Configurator) generateSetReqs() {
	props, np := &c.target.Props, &c.np
	if v, ok := props.GetMinElevation(); ok {
		deg := min(max(int((v+gpsprot.Degrees/2)/gpsprot.Degrees), 0), 90)
		c.addReq(fmt.Sprintf("setElevationMask, PVT, %d", deg), np.parseElevationMask)
	}
	if cmd := c.ppsSetCmd(); cmd != "" {
		c.addReq(cmd, np.parsePPSParameters)
	}
	if v, ok := props.GetAntennaCableDelay(); ok {
		ns := min(max(float64(v)/float64(time.Nanosecond), -10000), 10000)
		c.addReq("setCalibCommonDelay, "+formatFloat(ns), np.parseCalibCommonDelay)
	}
	if v, ok := props.GetRTCMBaseID(); ok {
		c.addReq(fmt.Sprintf("setRTCMv3Formatting, %d", min(v, 4095)), np.parseRTCMv3Formatting)
	}
	if v, ok := props.GetNavMsgAuth(); ok && c.caps.caps["GalOSNMA"] {
		// Loose is the owner-ruled interim level; strict needs trusted time
		// (exeSetTime) and follows the osnma branch. MTRoot is always left
		// alone (blank in live operation); disabling clears MeasLevel too,
		// since an active MeasLevel alone still applies authentication.
		if v == gpsprot.NavMsgAuthOSNMA {
			c.addReq("setGalOSNMAUsage, loose", np.parseGalOSNMAUsage)
		} else {
			c.addReq("setGalOSNMAUsage, off, , off", np.parseGalOSNMAUsage)
		}
	}
}

// ppsSetCmd builds the setPPSParameters command for the target's TimePulse
// and TimeGNSS properties, omitting arguments that keep their current value.
// Returns "" when the target sets none of them.
func (c *Configurator) ppsSetCmd() string {
	props, np := &c.target.Props, &c.np
	if w, ok := props.GetTimePulseWidth(); ok && w == 0 {
		return "setPPSParameters, off"
	}
	// Args: Interval, Polarity, Delay, TimeScale, MaxHoldover, PulseWidth.
	// Delay moves only the pulse, not the pseudoranges: never touched
	// (AntennaCableDelay is setCalibCommonDelay).
	args := make([]string, 6)
	if p, ok := props.GetTimePulsePeriod(); ok {
		args[0] = nearestPPSInterval(p)
	}
	if r, ok := props.GetTimePulsePolarityRising(); ok {
		if r {
			args[1] = "Low2High"
		} else {
			args[1] = "High2Low"
		}
	}
	if g, ok := props.GetTimeGNSS(); ok {
		args[3] = gnssTimeScale[g] // unmapped GNSS leaves TimeScale as is
	} else if align, ok := props.GetTimePulseAlignToGNSS(); ok {
		if !align {
			args[3] = "RxClock"
		} else if np.pps == nil || np.pps.timeScale == "RxClock" {
			args[3] = "GPS"
		}
	}
	if locked, ok := props.GetTimePulseOnlyWhenLocked(); ok {
		if locked {
			args[4] = "1"
		} else {
			args[4] = "0"
		}
	}
	if w, ok := props.GetTimePulseWidth(); ok {
		ms := min(max(float64(w)/float64(time.Millisecond), 0.000001), 1000)
		args[5] = formatFloat(ms)
		if args[0] == "" && (c.np.pps == nil || c.np.pps.interval == "off") {
			// A width alone cannot enable a disabled pulse; supply the
			// default 1 s interval.
			args[0] = "sec1"
		}
	}
	last := -1
	for i, a := range args {
		if a != "" {
			last = i
		}
	}
	if last < 0 {
		return ""
	}
	return "setPPSParameters, " + strings.Join(args[:last+1], ", ")
}

// nearestPPSInterval returns the Interval enum value whose period is closest
// to d: the receiver's interval choices are fixed, and a non-representable
// period is realized best-effort with the truth coming from the readback.
func nearestPPSInterval(d time.Duration) string {
	best, bestDiff := "", time.Duration(0)
	for name, period := range ppsIntervals {
		diff := (period - d).Abs()
		if best == "" || diff < bestDiff || (diff == bestDiff && period < ppsIntervals[best]) {
			best, bestDiff = name, diff
		}
	}
	return best
}

// formatFloat formats a float argument in the shortest exact decimal form.
func formatFloat(f float64) string {
	return strconv.FormatFloat(f, 'f', -1, 64)
}
