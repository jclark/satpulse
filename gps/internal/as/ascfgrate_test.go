package as

import (
	"reflect"
	"testing"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/asbin"
)

// tau1302Ver is a 1Hz, RTCM-capable unit (HD9310); the TAU1201 identity
// does not claim RTCM.
func tau1302Ver() *asbin.MonVer {
	return &asbin.MonVer{
		SwVersion: z16("3.018.f3667a12"),
		HwVersion: z16("HD9310.92257eed4"),
	}
}

// leapSecTarget turns on NAV-TIME only.
func leapSecTarget() *gpsprot.ConfigTarget {
	target := &gpsprot.ConfigTarget{}
	target.Opts.PVTMsg.Set(gpsprot.PVTMsgLeapSecond)
	return target
}

// leapSecOnlyTarget turns on NAV-TIME and turns the other PVT messages off.
func leapSecOnlyTarget() *gpsprot.ConfigTarget {
	target := &gpsprot.ConfigTarget{}
	target.Opts.PVTMsg.Set(gpsprot.PVTMsgLeapSecond | gpsprot.PVTMsgOff)
	return target
}

// rmcTarget turns on RMC and turns the other NMEA sentences off, and
// saves.
func rmcTarget() *gpsprot.ConfigTarget {
	target := &gpsprot.ConfigTarget{}
	target.Opts.NMEAMsg.Set(gpsprot.NMEAMsgRMC)
	target.Opts.Save = gpsprot.SaveAll
	return target
}

func sets(period uint8, mids ...asbin.MsgID) []cfgSet {
	var out []cfgSet
	for _, mid := range mids {
		out = append(out, cfgSet{mid, period})
	}
	return out
}

func TestPeriodSearch(t *testing.T) {
	type result struct {
		Errs  int
		Sets  []cfgSet
		Saves int
	}
	navTime := asbin.NavTimeID
	pvtOffs := sets(0, asbin.NavPosLlhID, asbin.NavPosEcefID, asbin.NavVelNedID, asbin.NavVelEcefID,
		asbin.NavTimeUtcID, asbin.NavDopID, asbin.NavAutoID, asbin.NavSvinID)
	nmeaOffs := sets(0, asbin.NmeaGsvID, asbin.NmeaGgaID, asbin.NmeaGsaID, asbin.NmeaZdaID,
		asbin.NmeaVtgID, asbin.NmeaGllID, asbin.NmeaGstID, asbin.NmeaGrsID, asbin.NmeaDtmID, asbin.NmeaJamID)
	join := func(parts ...[]cfgSet) []cfgSet {
		var out []cfgSet
		for _, p := range parts {
			out = append(out, p...)
		}
		return out
	}
	tests := []struct {
		name   string
		rcvr   testReceiver
		target *gpsprot.ConfigTarget
		expect result
	}{
		{
			// the usual case: the message is arriving once per second, so
			// its Period is the answer, and it is set even though it is
			// already right
			name:   "5Hz configured earlier",
			rcvr:   testReceiver{hz: 5, rates: map[asbin.MsgID]uint8{navTime: 5}},
			target: leapSecTarget(),
			expect: result{Sets: sets(5, navTime)},
		},
		{
			name:   "5Hz configured earlier, 4-byte poll reply",
			rcvr:   testReceiver{hz: 5, rates: map[asbin.MsgID]uint8{navTime: 5}, fourByteCfgMsg: true},
			target: leapSecTarget(),
			expect: result{Sets: sets(5, navTime)},
		},
		{
			name:   "1Hz configured earlier",
			rcvr:   testReceiver{hz: 1, rates: map[asbin.MsgID]uint8{navTime: 1}},
			target: leapSecTarget(),
			expect: result{Sets: sets(1, navTime)},
		},
		{
			name:   "10Hz configured earlier",
			rcvr:   testReceiver{hz: 10, rates: map[asbin.MsgID]uint8{navTime: 10}},
			target: leapSecTarget(),
			expect: result{Sets: sets(10, navTime)},
		},
		{
			// another message supplies the answer for one that is off
			name:   "answer from a different message",
			rcvr:   testReceiver{hz: 5, rates: map[asbin.MsgID]uint8{asbin.NmeaRmcID: 5}},
			target: leapSecTarget(),
			expect: result{Sets: sets(5, navTime)},
		},
		{
			// turn-offs wait for the look stage, then go out before the turn-on
			name:   "turn-offs with a turn-on",
			rcvr:   testReceiver{hz: 5, rates: map[asbin.MsgID]uint8{navTime: 5}},
			target: leapSecOnlyTarget(),
			expect: result{Sets: join(pvtOffs, sets(5, navTime))},
		},
		{
			// everything too fast: the 200ms gaps suggest 5, which is
			// tried first and confirmed
			name:   "5Hz with the message at Period 1",
			rcvr:   testReceiver{hz: 5, rates: map[asbin.MsgID]uint8{navTime: 1}},
			target: leapSecTarget(),
			expect: result{Sets: sets(5, navTime)},
		},
		{
			name:   "10Hz with the message at Period 1",
			rcvr:   testReceiver{hz: 10, rates: map[asbin.MsgID]uint8{navTime: 1}},
			target: leapSecTarget(),
			expect: result{Sets: sets(10, navTime)},
		},
		{
			// the hint is only a guess: Period 2 at 10Hz also gives 200ms
			name:   "10Hz with the message at Period 2",
			rcvr:   testReceiver{hz: 10, rates: map[asbin.MsgID]uint8{navTime: 2}},
			target: leapSecTarget(),
			expect: result{Sets: []cfgSet{{navTime, 5}, {navTime, 10}}},
		},
		{
			// nothing to go on: largest Period first, so no trial sends
			// the message more than once per second
			name:   "quiet 5Hz",
			rcvr:   testReceiver{hz: 5},
			target: leapSecTarget(),
			expect: result{Sets: []cfgSet{{navTime, 10}, {navTime, 5}}},
		},
		{
			name:   "quiet 1Hz",
			rcvr:   testReceiver{hz: 1},
			target: leapSecTarget(),
			expect: result{Sets: []cfgSet{{navTime, 10}, {navTime, 5}, {navTime, 1}}},
		},
		{
			name:   "quiet 10Hz",
			rcvr:   testReceiver{hz: 10},
			target: leapSecTarget(),
			expect: result{Sets: sets(10, navTime)},
		},
		{
			// 800ms gaps pass as about a second, but Period 4 is not an
			// answer, so the message is ignored and the trials decide
			name:   "5Hz with the message at Period 4",
			rcvr:   testReceiver{hz: 5, rates: map[asbin.MsgID]uint8{navTime: 4}},
			target: leapSecTarget(),
			expect: result{Sets: []cfgSet{{navTime, 10}, {navTime, 5}}},
		},
		{
			// no requested message can be the probe, so NAV-TIME is
			// borrowed and turned off again
			name:   "quiet 5Hz, NMEA only",
			rcvr:   testReceiver{hz: 5},
			target: rmcTarget(),
			expect: result{Sets: join(nmeaOffs, []cfgSet{{navTime, 10}, {navTime, 5}, {asbin.NmeaRmcID, 5}, {navTime, 0}}), Saves: 1},
		},
		{
			// a borrowed probe that was already on goes back to its Period
			name:   "5Hz, NMEA only, NAV-TIME at Period 1",
			rcvr:   testReceiver{hz: 5, rates: map[asbin.MsgID]uint8{navTime: 1}},
			target: rmcTarget(),
			expect: result{Sets: join(nmeaOffs, []cfgSet{{navTime, 5}, {asbin.NmeaRmcID, 5}, {navTime, 1}}), Saves: 1},
		},
		{
			// no trial arrives once per second: nothing is turned on, the
			// probe is put back, and nothing is saved
			name:   "receiver sends nothing",
			rcvr:   testReceiver{hz: 5, mute: true},
			target: rmcTarget(),
			expect: result{Errs: 1, Sets: join(nmeaOffs, []cfgSet{{navTime, 10}, {navTime, 5}, {navTime, 1}, {navTime, 0}})},
		},
		{
			name:   "receiver dropping output",
			rcvr:   testReceiver{hz: 5, rates: map[asbin.MsgID]uint8{navTime: 5}, overflowing: true},
			target: rmcTarget(),
			expect: result{Errs: 1, Sets: nmeaOffs},
		},
		{
			name:   "probe refused",
			rcvr:   testReceiver{hz: 5, nakTargets: map[asbin.MsgID]bool{navTime: true}},
			target: leapSecTarget(),
			expect: result{Errs: 1},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rcvr := tc.rcvr
			rcvr.monVer = tau951mVer()
			_, errs := configure(t, probe(t, &rcvr), &rcvr, tc.target)
			got := result{Errs: errs, Sets: rcvr.sets, Saves: len(rcvr.saves)}
			if !reflect.DeepEqual(got, tc.expect) {
				t.Errorf("got  %+v\nwant %+v", got, tc.expect)
			}
		})
	}
}

func TestPeriodSearchIrregularStart(t *testing.T) {
	// A 1Hz receiver stepping its epochs as the TAU1201 does at first fix
	// (800ms then 500ms, then slewing at 980ms): nothing is concluded
	// from the short gaps, and the answer is 1.
	stamps := []uint32{0, 1000, 1800, 2300, 3280, 4260, 5240, 6220, 7200, 8190}
	rcvr := &testReceiver{monVer: tau1201Ver(), hz: 1, rates: map[asbin.MsgID]uint8{asbin.NavTimeID: 1}}
	rcvr.stamp = func(cycle int) uint32 {
		if cycle < len(stamps) {
			return 35223790 + stamps[cycle]
		}
		return 35232000 + uint32(cycle-len(stamps))*1000
	}
	_, errs := configure(t, probe(t, rcvr), rcvr, leapSecTarget())
	if errs != 0 {
		t.Errorf("errors = %d, want 0", errs)
	}
	if expect := sets(1, asbin.NavTimeID); !reflect.DeepEqual(rcvr.sets, expect) {
		t.Errorf("got  %+v\nwant %+v", rcvr.sets, expect)
	}
}
