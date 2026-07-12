package as

import (
	"fmt"
	"testing"
	"time"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/internal/nmea"
	"github.com/jclark/satpulse/gps/lib/asbin"
)

// navEmitter emits NAV-TIME from a testReceiver at its native cycle. The
// receiver only emits an enabled message, and at its stored divisor:
// once per stored-rate native cycles. A 5Hz-native unit (periodMs=200)
// with the message at rate=1 therefore emits every 200ms (the bug), and
// at rate=5 every 1000ms (the fix). silent models no fix: no emission
// even when enabled.
type navEmitter struct {
	rcvr     *testReceiver
	periodMs int
	baseTOW  uint32
	start    time.Time
	tick     int
	silent   bool
	itowStep []int  // per-emit iTOW deltas (ms); when set, iTOW is off-grid
	itow     uint32 // running iTOW offset for the off-grid path
	emitted  int    // count of off-grid emits, indexes itowStep
}

// due returns the NAV-TIME packets emitted up to now, advancing the
// emitter's native clock. iTOW advances by one native period each tick;
// the message is emitted only on ticks that are a multiple of its
// current stored divisor.
func (ne *navEmitter) due(now time.Time) [][]byte {
	if ne.silent {
		return nil
	}
	var out [][]byte
	for {
		tickTime := ne.start.Add(time.Duration(ne.tick*ne.periodMs) * time.Millisecond)
		if tickTime.After(now) {
			break
		}
		rate := int(ne.rcvr.rates[asbin.NavTimeID])
		if rate > 0 && ne.tick%rate == 0 {
			tow := ne.baseTOW + uint32(ne.tick*ne.periodMs)
			if len(ne.itowStep) > 0 {
				ne.itow += uint32(ne.itowStep[ne.emitted%len(ne.itowStep)])
				tow = ne.baseTOW + ne.itow
				ne.emitted++
			}
			pkt, _ := asbin.Serialize(&asbin.NavTime{RefTow: tow})
			out = append(out, pkt)
		}
		ne.tick++
	}
	return out
}

// configureTraffic runs the full director loop against a receiver that
// also emits periodic NAV traffic, so the rate estimator observes it.
// As-found traffic is pre-seeded for the second before configuring.
func configureTraffic(t *testing.T, cp *ConfigProtocol, rcvr *testReceiver, target *gpsprot.ConfigTarget, em *navEmitter) (*Configurator, int) {
	pp := NewPacketProcessor(gpsprot.NewNavEpochManager())
	pp.SetNativeMsgHandler(cp)
	cfgI, err := cp.Configure(target)
	if err != nil {
		t.Fatalf("Configure: %v", err)
	}
	cfg := cfgI.(*Configurator)
	director := gpsprot.NewConfigDirector(cfg, 2)
	t0 := time.Date(2026, 1, 1, 0, 1, 0, 0, time.UTC)
	em.start = t0.Add(-time.Second)
	feed := func(now time.Time) {
		for _, pkt := range em.due(now) {
			if _, err := pp.ProcessPacket(string(pkt), now); err != nil {
				t.Fatalf("ProcessPacket(emit): %v", err)
			}
			director.ValidPacketReceived(now)
		}
	}
	feed(t0) // pre-seed as-found traffic
	for action := range director.Actions() {
		switch action.Type {
		case gpsprot.ConfigActionSendRequest:
			t0 = t0.Add(10 * time.Millisecond)
			feed(t0)
			cfg.Request(action.Index).SetSentTime(t0)
			for _, resp := range append(rcvr.respond(action.Packet), rcvr.takePending()...) {
				t0 = t0.Add(5 * time.Millisecond)
				if _, err := pp.ProcessPacket(string(resp), t0); err != nil {
					t.Fatalf("ProcessPacket: %v", err)
				}
				director.ValidPacketReceived(t0)
			}
			feed(t0)
		case gpsprot.ConfigActionWaitUntil:
			if action.Deadline.After(t0) {
				t0 = action.Deadline.Add(time.Millisecond)
			}
			feed(t0)
			director.AdvanceTimeTo(t0)
		}
	}
	return cfg, director.ErrorCount
}

func leapSecTarget() *gpsprot.ConfigTarget {
	target := &gpsprot.ConfigTarget{}
	target.Opts.PVTMsg.Set(gpsprot.PVTMsgLeapSecond) // enables NAV-TIME only
	return target
}

func TestRateRestartLegacyBuggy(t *testing.T) {
	// A 5Hz-native unit left by an old (buggy) run with NAV-TIME at the
	// factory divisor 1, so it emits at 5Hz. The query-phase poll returns
	// R=1 and the observed 200ms interval gives native=5, so the enable
	// goes out at rate=5 - correct first try.
	rcvr := &testReceiver{monVer: tau1201Ver(), rates: map[asbin.MsgID]uint8{asbin.NavTimeID: 1}}
	cp := probe(t, rcvr)
	em := &navEmitter{rcvr: rcvr, periodMs: 200, baseTOW: 100000}
	cfg, errCount := configureTraffic(t, cp, rcvr, leapSecTarget(), em)
	if errCount != 0 {
		t.Errorf("ErrorCount = %d, want 0", errCount)
	}
	if cp.est.nativeHz != 5 {
		t.Errorf("nativeHz = %d, want 5", cp.est.nativeHz)
	}
	if rcvr.rates[asbin.NavTimeID] != 5 {
		t.Errorf("NAV-TIME rate = %d, want 5", rcvr.rates[asbin.NavTimeID])
	}
	_ = cfg
}

func TestRateRestartCorrectSkip(t *testing.T) {
	// A 1Hz unit already emitting NAV-TIME at 1Hz: the observed 1s
	// interval means it is already doing the requested thing, so the
	// enable is skipped (no CFG-MSG set at all).
	rcvr := &testReceiver{monVer: tau1201Ver(), rates: map[asbin.MsgID]uint8{asbin.NavTimeID: 1}}
	cp := probe(t, rcvr)
	em := &navEmitter{rcvr: rcvr, periodMs: 1000, baseTOW: 100000}
	_, errCount := configureTraffic(t, cp, rcvr, leapSecTarget(), em)
	if errCount != 0 {
		t.Errorf("ErrorCount = %d, want 0", errCount)
	}
	if cp.est.nativeHz != 1 {
		t.Errorf("nativeHz = %d, want 1", cp.est.nativeHz)
	}
	for _, mid := range rcvr.msgSets {
		if mid == asbin.NavTimeID {
			t.Errorf("NAV-TIME was set (rate=%d); an already-1Hz message must be skipped", rcvr.rates[asbin.NavTimeID])
		}
	}
}

func TestRateSilentThenFlowing(t *testing.T) {
	// A 5Hz unit with NAV-TIME initially off (nothing to observe). The
	// enable is deferred, then sent at rate=1 to create evidence; the
	// message betrays itself at 200ms, and the correction re-issues it at
	// rate=5.
	rcvr := &testReceiver{monVer: tau1201Ver()}
	cp := probe(t, rcvr)
	em := &navEmitter{rcvr: rcvr, periodMs: 200, baseTOW: 100000}
	_, errCount := configureTraffic(t, cp, rcvr, leapSecTarget(), em)
	if errCount != 0 {
		t.Errorf("ErrorCount = %d, want 0", errCount)
	}
	if cp.est.nativeHz != 5 {
		t.Errorf("nativeHz = %d, want 5", cp.est.nativeHz)
	}
	if rcvr.rates[asbin.NavTimeID] != 5 {
		t.Errorf("NAV-TIME rate = %d, want 5 (corrected)", rcvr.rates[asbin.NavTimeID])
	}
}

// hasCfgMsgSet reports whether cfg has queued a CFG-MSG set of mid to rate.
func hasCfgMsgSet(cfg *Configurator, mid asbin.MsgID, rate uint8) bool {
	cls, id := mid.Unpack()
	for _, req := range cfg.reqs {
		if req.packet == nil {
			continue
		}
		m, err := asbin.ParseMsg(string(req.packet))
		if err != nil {
			continue
		}
		if cm, ok := m.(*asbin.CfgMsg); ok && cm.MsgClass == cls && cm.MsgID == id && cm.Rate == rate {
			return true
		}
	}
	return false
}

func TestRateFlowingWatchFallback(t *testing.T) {
	// Traffic is flowing but the rate cannot be computed yet (the poll was
	// orphaned): the resolve phase installs a flowing watch that sends
	// nothing. If the wait elapses without resolution it falls back to the
	// silent path - enable at rate=1 to create evidence - and the
	// self-set interval then resolves the rate and corrects the enable.
	est := newRateEstimator()
	feedNav(est, 1000, 1200) // NAV-POSLLH flowing at 200ms, unresolved
	if !est.sawPeriodic() || est.nativeHz != 0 {
		t.Fatalf("setup: sawPeriodic=%v nativeHz=%d", est.sawPeriodic(), est.nativeHz)
	}
	cfg := newConfigurator(&gpsprot.ConfigTarget{}, tau1201Ver(), est)
	cfg.deferred = []deferredEnable{{mid: asbin.NavTimeID, nakOK: true}}

	cfg.generateResolveReqs()
	if cfg.watch == nil || cfg.watch.watchDur != resolveFlowingDelay {
		t.Fatalf("want a flowing watch, got %+v", cfg.watch)
	}
	if len(cfg.reqs) != 1 {
		t.Fatalf("a flowing watch must send nothing yet; reqs = %d", len(cfg.reqs))
	}

	cfg.watchTimeout(cfg.watch) // the flowing wait elapses
	if !cfg.watchSent || cfg.watch.watchDur != resolveSilentDelay {
		t.Fatalf("flowing->silent failed: watchSent=%v dur=%v", cfg.watchSent, cfg.watch.watchDur)
	}
	if !hasCfgMsgSet(cfg, asbin.NavTimeID, 1) {
		t.Fatal("flowing->silent must enable NAV-TIME at rate=1")
	}

	// NAV-TIME (now self-set) betrays a 5Hz native rate.
	t0 := time.Date(2026, 1, 1, 0, 2, 0, 0, time.UTC)
	for _, w := range []uint32{2000, 2200, 2400} {
		est.observe(&asbin.NavTime{RefTow: w}, t0)
		t0 = t0.Add(time.Second)
	}
	cfg.onObserve()
	if est.nativeHz != 5 {
		t.Fatalf("nativeHz = %d, want 5", est.nativeHz)
	}
	if !hasCfgMsgSet(cfg, asbin.NavTimeID, 5) {
		t.Error("the watch must re-issue NAV-TIME at rate=5")
	}
	if cfg.watch != nil {
		t.Error("the watch should be resolved")
	}
}

func TestRateRestartLegacyBuggy5Hz(t *testing.T) {
	// The real 5Hz units (TAU951M-P200, D10P) answer a CFG-MSG poll with
	// the 4-byte form (rate + 0xFF port mask), so pair a 5Hz scenario with
	// that hardware form and identity. Left by an old buggy run with
	// NAV-TIME at divisor 1, it emits at 5Hz; the poll returns R=1 and the
	// observed 200ms interval gives native=5, so the enable goes out at
	// rate=5 - correct first try.
	rcvr := &testReceiver{monVer: tau951mVer(), fourByteCfgMsg: true,
		rates: map[asbin.MsgID]uint8{asbin.NavTimeID: 1}}
	cp := probe(t, rcvr)
	em := &navEmitter{rcvr: rcvr, periodMs: 200, baseTOW: 100000}
	_, errCount := configureTraffic(t, cp, rcvr, leapSecTarget(), em)
	if errCount != 0 {
		t.Errorf("ErrorCount = %d, want 0", errCount)
	}
	if cp.est.nativeHz != 5 {
		t.Errorf("nativeHz = %d, want 5", cp.est.nativeHz)
	}
	if rcvr.rates[asbin.NavTimeID] != 5 {
		t.Errorf("NAV-TIME rate = %d, want 5", rcvr.rates[asbin.NavTimeID])
	}
}

func TestRateChattyUnresolvable(t *testing.T) {
	// Continuous non-resolving traffic must not postpone the cap forever.
	// NAV-TIME is initially off, so the enable is deferred and sent at
	// rate=1; the unit then emits it OFF-GRID (inconsistent iTOW deltas, as
	// a fixless receiver does), so rule 2 never resolves. The watch's
	// deadline is anchored, not floated on this stream, so it caps in
	// bounded time, concludes 1Hz, and the invocation completes rather than
	// hanging.
	rcvr := &testReceiver{monVer: tau1201Ver()}
	cp := probe(t, rcvr)
	em := &navEmitter{rcvr: rcvr, periodMs: 100, baseTOW: 100000,
		itowStep: []int{60, 200}} // no two consecutive deltas ever agree
	_, errCount := configureTraffic(t, cp, rcvr, leapSecTarget(), em)
	if errCount != 0 {
		t.Errorf("ErrorCount = %d, want 0", errCount)
	}
	if cp.est.nativeHz != 1 {
		t.Errorf("nativeHz = %d, want 1 (anchored negative over non-resolving traffic)", cp.est.nativeHz)
	}
	if rcvr.rates[asbin.NavTimeID] != 1 {
		t.Errorf("NAV-TIME rate = %d, want 1", rcvr.rates[asbin.NavTimeID])
	}
}

func TestRateDeadSilentCap(t *testing.T) {
	// A receiver that never emits (no fix): the enable goes out at rate=1
	// and the cap concludes 1Hz, flagged as unverified.
	rcvr := &testReceiver{monVer: tau1201Ver()}
	cp := probe(t, rcvr)
	em := &navEmitter{rcvr: rcvr, periodMs: 200, baseTOW: 100000, silent: true}
	_, errCount := configureTraffic(t, cp, rcvr, leapSecTarget(), em)
	if errCount != 0 {
		t.Errorf("ErrorCount = %d, want 0", errCount)
	}
	if cp.est.nativeHz != 1 || !cp.est.capped {
		t.Errorf("nativeHz = %d capped = %v, want 1 and true", cp.est.nativeHz, cp.est.capped)
	}
	if rcvr.rates[asbin.NavTimeID] != 1 {
		t.Errorf("NAV-TIME rate = %d, want 1 (optimistic)", rcvr.rates[asbin.NavTimeID])
	}
}

// nmeaEmitter emits one time-bearing NMEA sentence (RMC) per 1Hz output
// epoch, its time-of-day stepping one second. A receiver's NMEA output
// cadence is 1Hz whatever its native cycle; the native rate shows only in
// the stored CFG-MSG divisor a poll reveals (5 on a 5Hz unit, 1 on a 1Hz
// unit), not in the output spacing. This is the as-found traffic of the
// RTCM-out scenario: NMEA the request leaves alone, flowing beside the
// enabled RTCM targets that the estimator cannot see.
type nmeaEmitter struct {
	start time.Time
	tick  int
}

// due returns the RMC sentences emitted up to now. Each carries a
// time-of-day one second past the last, so the estimator sees a clean
// 1000ms content-time interval.
func (ne *nmeaEmitter) due(t *testing.T, now time.Time) []*nmea.Sentence {
	var out []*nmea.Sentence
	for {
		tickTime := ne.start.Add(time.Duration(ne.tick) * time.Second)
		if tickTime.After(now) {
			break
		}
		out = append(out, rmcSentence(t, fmt.Sprintf("%06d.00", 10000+ne.tick)))
		ne.tick++
	}
	return out
}

// rtcmOutTarget requests RTCM MSM4 + ARP output and nothing else - the
// invocation shape that exposed the lazy-poll defect. The enabled RTCM
// targets are invisible to the rate estimator (a separate processor), so
// only the as-found NMEA traffic can resolve the native rate.
func rtcmOutTarget() *gpsprot.ConfigTarget {
	target := &gpsprot.ConfigTarget{}
	target.Opts.RTCMMsg.Set(gpsprot.RTCMMsgMSM4 | gpsprot.RTCMMsgARP)
	return target
}

// tau1302Ver is a 1Hz-native, RTCM-capable unit (HD9310), for the 1Hz
// leg of the lazy-poll test - the TAU1201 identity does not claim RTCM.
func tau1302Ver() *asbin.MonVer {
	return &asbin.MonVer{
		SwVersion: z16("3.018.f3667a12"),
		HwVersion: z16("HD9310.92257eed4"),
	}
}

// configureLazyNMEA runs the director loop against a receiver whose only
// visible traffic is as-found NMEA fed straight to the ConfigProtocol's
// HandledNativeMsg (exactly as the nmea processor does for a consumed
// time-bearing sentence). It seeds ONE arrival before configuring so the
// query-phase eager poll cannot qualify, and steps each wait in small
// increments so a request queued mid-wait - the lazy poll - is
// transmitted and answered before the watch deadline, the way the real
// driver reacts to every packet.
func configureLazyNMEA(t *testing.T, cp *ConfigProtocol, rcvr *testReceiver, target *gpsprot.ConfigTarget, em *nmeaEmitter) (*Configurator, int) {
	pp := NewPacketProcessor(gpsprot.NewNavEpochManager())
	pp.SetNativeMsgHandler(cp)
	cfgI, err := cp.Configure(target)
	if err != nil {
		t.Fatalf("Configure: %v", err)
	}
	cfg := cfgI.(*Configurator)
	director := gpsprot.NewConfigDirector(cfg, 2)
	t0 := time.Date(2026, 1, 1, 0, 1, 0, 0, time.UTC)
	em.start = t0
	feed := func(now time.Time) {
		for _, sen := range em.due(t, now) {
			if err := cp.HandledNativeMsg(nmea.Tag, sen.AddressField(), sen, now); err != nil {
				t.Fatalf("HandledNativeMsg(emit): %v", err)
			}
			director.ValidPacketReceived(now)
		}
	}
	feed(t0) // one pre-seed arrival: too little for sawPeriodic
	const step = 250 * time.Millisecond
	for action := range director.Actions() {
		switch action.Type {
		case gpsprot.ConfigActionSendRequest:
			t0 = t0.Add(10 * time.Millisecond)
			feed(t0)
			cfg.Request(action.Index).SetSentTime(t0)
			for _, resp := range append(rcvr.respond(action.Packet), rcvr.takePending()...) {
				t0 = t0.Add(5 * time.Millisecond)
				if _, err := pp.ProcessPacket(string(resp), t0); err != nil {
					t.Fatalf("ProcessPacket: %v", err)
				}
				director.ValidPacketReceived(t0)
			}
			feed(t0)
		case gpsprot.ConfigActionWaitUntil:
			if next := t0.Add(step); next.Before(action.Deadline) {
				t0 = next
			} else {
				t0 = action.Deadline.Add(time.Millisecond)
			}
			feed(t0)
			director.AdvanceTimeTo(t0)
		}
	}
	return cfg, director.ErrorCount
}

func TestRateLazyPollRTCMOut5Hz(t *testing.T) {
	// The hardware failure: a 5Hz-native unit (RMC stored at divisor 5, so
	// NMEA flows at 1Hz) configured for RTCM MSM4+ARP output only. NMEA is
	// left alone and keeps flowing, but the enabled RTCM targets are
	// invisible to the estimator. Only one NMEA epoch precedes the resolve
	// phase, so the eager poll never qualifies and the enables are deferred
	// then sent at rate=1 (the silent path). The lazy poll fires on the
	// second NMEA arrival, its 4-byte readback (R=5) pairs with the observed
	// 1000ms interval to resolve native=5, and the deferred RTCM enables are
	// corrected to rate=5 - so RTCM emits at 1Hz, not 5Hz.
	rcvr := &testReceiver{monVer: tau951mVer(), fourByteCfgMsg: true,
		rates: map[asbin.MsgID]uint8{asbin.NmeaRmcID: 5}}
	cp := probe(t, rcvr)
	em := &nmeaEmitter{}
	cfg, errCount := configureLazyNMEA(t, cp, rcvr, rtcmOutTarget(), em)
	if errCount != 0 {
		t.Errorf("ErrorCount = %d, want 0", errCount)
	}
	if !cfg.ratePollSent || !cp.est.tracks[asbin.NmeaRmcID].hasPoll {
		t.Errorf("lazy poll did not go out and get answered: ratePollSent=%v hasPoll=%v",
			cfg.ratePollSent, cp.est.tracks[asbin.NmeaRmcID].hasPoll)
	}
	if cp.est.nativeHz != 5 {
		t.Errorf("nativeHz = %d, want 5", cp.est.nativeHz)
	}
	if rcvr.rates[asbin.RtcmMsm4GpsID] != 5 || rcvr.rates[asbin.RtcmArpID] != 5 {
		t.Errorf("RTCM rates MSM4=%d ARP=%d, want 5 (corrected)",
			rcvr.rates[asbin.RtcmMsm4GpsID], rcvr.rates[asbin.RtcmArpID])
	}
}

func TestRateLazyPollRTCMOut1Hz(t *testing.T) {
	// The same shape on a 1Hz unit (RMC stored at divisor 1): the lazy poll
	// resolves native=1, and because the deferred enables were already sent
	// at rate=1 the correction is a no-op - no re-issue. RTCM stays at
	// rate=1, correct for a 1Hz unit.
	rcvr := &testReceiver{monVer: tau1302Ver(),
		rates: map[asbin.MsgID]uint8{asbin.NmeaRmcID: 1}}
	cp := probe(t, rcvr)
	em := &nmeaEmitter{}
	cfg, errCount := configureLazyNMEA(t, cp, rcvr, rtcmOutTarget(), em)
	if errCount != 0 {
		t.Errorf("ErrorCount = %d, want 0", errCount)
	}
	if !cfg.ratePollSent || !cp.est.tracks[asbin.NmeaRmcID].hasPoll {
		t.Errorf("lazy poll did not go out and get answered: ratePollSent=%v hasPoll=%v",
			cfg.ratePollSent, cp.est.tracks[asbin.NmeaRmcID].hasPoll)
	}
	if cp.est.nativeHz != 1 {
		t.Errorf("nativeHz = %d, want 1", cp.est.nativeHz)
	}
	if rcvr.rates[asbin.RtcmMsm4GpsID] != 1 {
		t.Errorf("RTCM MSM4 rate = %d, want 1", rcvr.rates[asbin.RtcmMsm4GpsID])
	}
	if n := countCfgMsgSets(rcvr, asbin.RtcmMsm4GpsID); n != 1 {
		t.Errorf("RTCM MSM4 was set %d times, want 1 (no rate correction on a 1Hz unit)", n)
	}
}

// countCfgMsgSets counts how many CFG-MSG sets of mid the receiver saw.
func countCfgMsgSets(rcvr *testReceiver, mid asbin.MsgID) int {
	n := 0
	for _, m := range rcvr.msgSets {
		if m == mid {
			n++
		}
	}
	return n
}
