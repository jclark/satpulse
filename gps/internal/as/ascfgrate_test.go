package as

import (
	"testing"
	"time"

	"github.com/jclark/satpulse/gps/gpsprot"
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
