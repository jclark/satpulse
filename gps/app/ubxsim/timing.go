package ubxsim

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/jclark/satpulse/gps/internal/ubx"
	"github.com/jclark/satpulse/gps/lib/ubxbin"
)

// Timing supplies the independent reference clock and nominal pulse scheduler.
// ReadTime returns paired UTC Unix and CLOCK_MONOTONIC_RAW nanoseconds.
// ScheduleEdge replaces the schedule with one future rising-edge raw-clock
// deadline, not the time of the call. Each previous deadline has passed before
// this callback is called again.
// LeapSeconds is the fixed GPS-UTC offset; leap transitions are not simulated.
type Timing struct {
	ReadTime     func() (utc, raw int64, err error)
	ScheduleEdge func(raw int64) error
	LeapSeconds  int
}

const timingDelay = 100 * time.Millisecond

func (n *navEngine) runTiming(ctx context.Context, t *Timing) error {
	if t.ReadTime == nil || t.ScheduleEdge == nil || t.LeapSeconds < 0 || t.LeapSeconds > 127 {
		return fmt.Errorf("invalid timing options")
	}
	timer := time.NewTimer(50 * time.Millisecond)
	defer timer.Stop()
	var fit clockFit
	var pending int64
	var epoch uint64
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-n.reboot:
			epoch = 0
		case <-timer.C:
			timer.Reset(50 * time.Millisecond)
			utc, raw, err := t.ReadTime()
			if err != nil {
				return fmt.Errorf("read timing reference: %w", err)
			}
			if fit.add(utc, raw) {
				return fmt.Errorf("timing reference stepped or raw clock stopped")
			}
			if !fit.ready() {
				continue
			}
			if pending != 0 && utc >= pending+int64(timingDelay) {
				if utc >= pending+int64(time.Second) {
					return fmt.Errorf("timing output missed its second")
				}
				if err := n.emitTiming(ctx, time.Unix(0, pending).UTC(), t.LeapSeconds, epoch); err != nil {
					return err
				}
				epoch++
				pending = 0
			}
			if pending == 0 {
				pending = (utc/int64(time.Second) + 1) * int64(time.Second)
				if epoch == 0 && pending-utc < int64(250*time.Millisecond) {
					pending += int64(time.Second)
				}
				edge := fit.rawAt(pending)
				if err := t.ScheduleEdge(edge); err != nil {
					return fmt.Errorf("schedule timing edge: %w", err)
				}
				n.lg.Debug("timing edge scheduled", "utc", time.Unix(0, pending).UTC(), "raw", edge)
			}
			if d := time.Duration(pending + int64(timingDelay) - utc); d > 0 && d < 50*time.Millisecond {
				timer.Reset(d)
			}
		}
	}
}

func (n *navEngine) emitTiming(ctx context.Context, utc time.Time, leap int, epoch uint64) error {
	gen := n.w.generation()
	for _, m := range timingMessages(utc, leap) {
		// TIM-TOS belongs to M8 time/frequency products and has no F9
		// CFG-MSGOUT key; it cannot be enabled by this config engine.
		key, ok := msgOutKey[m.ID()]
		if !ok {
			continue
		}
		pkt := Pkt{KeyM: key, Tag: ubx.Tag}
		rate := n.db.ramUint(pkt.KeyM.KeyU(n.port).Key())
		if rate == 0 || epoch%rate != 0 || !n.enabled(pkt) {
			continue
		}
		b, err := ubxbin.Serialize(m)
		if err != nil {
			panic(err)
		}
		if err := n.w.sendGen(ctx, gen, b); err != nil {
			return err
		}
	}
	return nil
}

func timingMessages(utc time.Time, leap int) []ubxbin.Msg {
	week, tow := gpsTime(utc, leap)
	nextWeek, nextTOW := gpsTime(utc.Add(time.Second), leap)
	itow := ubxbin.NavITOW{ITOW: tow * 1000}
	y, m, d := uint16(utc.Year()), byte(utc.Month()), byte(utc.Day())
	h, min, sec := byte(utc.Hour()), byte(utc.Minute()), byte(utc.Second())
	return []ubxbin.Msg{
		&ubxbin.NavTimeGPS{NavITOW: itow, Week: int16(week), LeapS: byte(leap),
			Valid: ubxbin.NavTimeGPSTOWValid | ubxbin.NavTimeGPSWeekValid | ubxbin.NavTimeGPSLeapSValid, TAcc: 10},
		&ubxbin.NavTimeUTC{NavITOW: itow, Year: y, Month: m, Day: d, Hour: h, Min: min, Sec: sec,
			Valid: ubxbin.NavTimeUTCValidTOW | ubxbin.NavTimeUTCValidWkn | ubxbin.NavTimeUTCValidUTC | ubxbin.NavTimeUTCValid(ubxbin.UTCStandardUSNO<<4), TAcc: 10},
		&ubxbin.NavPVT{NavITOW: itow, Year: y, Month: m, Day: d, Hour: h, Min: min, Sec: sec,
			Valid:   ubxbin.NavPVTValidDate | ubxbin.NavPVTValidTime | ubxbin.NavPVTValidFullyResolved,
			Flags2:  ubxbin.NavPVTConfirmedAvai | ubxbin.NavPVTConfirmedDate | ubxbin.NavPVTConfirmedTime,
			FixType: ubxbin.NavPVTTimeOnlyFix, Flags: ubxbin.NavPVTGNSSFixOK, TAcc: 10},
		&ubxbin.TimTos{GNSSID: ubxbin.GPS, Year: y, Month: m, Day: d, Hour: h, Minute: min, Second: sec,
			UTCStandard: ubxbin.UTCStandardUSNO, Week: week, TOW: tow, UTCUncertainty: 10, GNSSUncertainty: 10,
			Flags: ubxbin.TimTosGNSSTimeValid | ubxbin.TimTosUTCTimeValid | ubxbin.TimTosTimeInLimit | ubxbin.TimTosLockedPulse | ubxbin.TimTosDiscSrcGNSS},
		&ubxbin.TimTP{Week: uint16(nextWeek), TOWMS: nextTOW * 1000,
			Flags: ubxbin.TimTPTimeBaseGNSS | ubxbin.TimTPUTCAvailable, RefInfo: ubxbin.TimTPTimeRefGPS},
	}
}

func gpsTime(utc time.Time, leap int) (week, tow uint32) {
	const gpsEpoch = 315964800
	const weekSeconds = 7 * 24 * 60 * 60
	s := utc.Unix() - gpsEpoch + int64(leap)
	return uint32(s / weekSeconds), uint32(s % weekSeconds)
}

// clockFit regresses raw time against UTC over the last five seconds.
// Integer origins preserve nanoseconds despite the large Unix epoch value.
type clockFit struct {
	samples [100]clockPair
	n, next int
}

type clockPair struct {
	utc, raw int64
}

func (f *clockFit) add(utc, raw int64) bool {
	if f.n != 0 {
		last := f.samples[(f.next+len(f.samples)-1)%len(f.samples)]
		if utc <= last.utc || raw <= last.raw {
			return true
		}
		if f.ready() && math.Abs(float64(raw-f.rawAt(utc))) > float64(time.Millisecond) {
			return true
		}
	}
	f.samples[f.next] = clockPair{utc, raw}
	f.next = (f.next + 1) % len(f.samples)
	if f.n < len(f.samples) {
		f.n++
	}
	return false
}

func (f *clockFit) ready() bool {
	if f.n < 20 {
		return false
	}
	first := 0
	if f.n == len(f.samples) {
		first = f.next
	}
	last := (f.next + len(f.samples) - 1) % len(f.samples)
	return f.samples[last].utc-f.samples[first].utc >= int64(900*time.Millisecond)
}

func (f *clockFit) rawAt(utc int64) int64 {
	o := f.samples[0]
	var sx, sy float64
	for _, p := range f.samples[:f.n] {
		sx += float64(p.utc - o.utc)
		sy += float64(p.raw - o.raw)
	}
	mx, my := sx/float64(f.n), sy/float64(f.n)
	var xx, xy float64
	for _, p := range f.samples[:f.n] {
		x, y := float64(p.utc-o.utc)-mx, float64(p.raw-o.raw)-my
		xx += x * x
		xy += x * y
	}
	return o.raw + int64(math.Round(my+(float64(utc-o.utc)-mx)*xy/xx))
}
