package septentrio

import (
	"bufio"
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/sbfbin"
)

// collector records every gpsprot.Msg and native message the processor emits.
type collector struct {
	gpsprot.DefaultHandler
	times   []*gpsprot.TimeMsg
	leaps   []*gpsprot.LeapSecondMsg
	posGeo  []*gpsprot.PosGeoMsg
	posECEF []*gpsprot.PosECEFMsg
	velGeo  []*gpsprot.VelGeoMsg
	velECEF []*gpsprot.VelECEFMsg
	sats    []*gpsprot.SatellitesMsg
	surveys []*gpsprot.SurveyMsg
	cors    []*gpsprot.CorReportMsg
	epochs  []*gpsprot.NavEpochMsg
	natives int
}

func (c *collector) Time(m *gpsprot.TimeMsg, _ time.Time)             { c.times = append(c.times, m) }
func (c *collector) LeapSecond(m *gpsprot.LeapSecondMsg, _ time.Time) { c.leaps = append(c.leaps, m) }
func (c *collector) PosGeo(m *gpsprot.PosGeoMsg, _ time.Time)         { c.posGeo = append(c.posGeo, m) }
func (c *collector) PosECEF(m *gpsprot.PosECEFMsg, _ time.Time)       { c.posECEF = append(c.posECEF, m) }
func (c *collector) VelGeo(m *gpsprot.VelGeoMsg, _ time.Time)         { c.velGeo = append(c.velGeo, m) }
func (c *collector) VelECEF(m *gpsprot.VelECEFMsg, _ time.Time)       { c.velECEF = append(c.velECEF, m) }
func (c *collector) Satellites(m *gpsprot.SatellitesMsg, _ time.Time) { c.sats = append(c.sats, m) }
func (c *collector) Survey(m *gpsprot.SurveyMsg, _ time.Time)         { c.surveys = append(c.surveys, m) }
func (c *collector) CorReport(m *gpsprot.CorReportMsg, _ time.Time)   { c.cors = append(c.cors, m) }
func (c *collector) NavEpoch(m *gpsprot.NavEpochMsg, _ time.Time)     { c.epochs = append(c.epochs, m) }

func (c *collector) NativeMsg(_ gpsprot.Tag, _ string, _ any, _ time.Time) error {
	c.natives++
	return nil
}

// newTestProcessor wires a processor to a fresh manager and collector.
func newTestProcessor() (*PacketProcessor, *collector) {
	c := &collector{}
	p := NewPacketProcessor(gpsprot.NewNavEpochManager())
	p.SetMsgHandler(c)
	p.SetNativeMsgHandler(c)
	return p, c
}

// captureRecord is one line of a *.jsonl packet capture.
type captureRecord struct {
	T   time.Time `json:"t"`
	Msg string    `json:"msg"`
	Bin string    `json:"bin"`
}

// readCapture returns the decoded binary packets from a capture file.
func readCapture(t *testing.T, name string) []captureRecord {
	t.Helper()
	f, err := os.Open("../../testdata/packets/septentrio/mosaic-G5/" + name)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var recs []captureRecord
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		var r captureRecord
		if json.Unmarshal(sc.Bytes(), &r) != nil || r.Bin == "" {
			continue
		}
		recs = append(recs, r)
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	return recs
}

// feed runs every packet in a capture through the processor.
func feed(t *testing.T, p *PacketProcessor, recs []captureRecord) {
	t.Helper()
	for _, r := range recs {
		data, err := hex.DecodeString(r.Bin)
		if err != nil {
			t.Fatalf("bad hex for %s: %v", r.Msg, err)
		}
		if _, err := p.ProcessPacket(string(data), r.T); err != nil {
			t.Fatalf("ProcessPacket(%s): %v", r.Msg, err)
		}
	}
}

// findBlock decodes the first packet of the named message type in a capture.
func findBlock(t *testing.T, capture, msg string) *sbfbin.Block {
	t.Helper()
	for _, r := range readCapture(t, capture) {
		if r.Msg != msg {
			continue
		}
		data, err := hex.DecodeString(r.Bin)
		if err != nil {
			t.Fatal(err)
		}
		b, err := sbfbin.ParseMsg(string(data))
		if err != nil {
			t.Fatalf("ParseMsg(%s): %v", msg, err)
		}
		return b
	}
	t.Fatalf("no %s block in %s", msg, capture)
	return nil
}

func testMeasBlock(ts sbfbin.TimeStamp, svid uint8, sig uint8, cn0 uint8) *sbfbin.Block {
	return &sbfbin.Block{TimeStamp: ts, Params: &sbfbin.MeasEpoch{
		Type1: []sbfbin.MeasEpochChannelType1{{SVID: svid, Type: sbfbin.MeasType(sig), CN0: cn0}},
		Type2: [][]sbfbin.MeasEpochChannelType2{{}},
	}}
}

func testChannelStatusBlock(ts sbfbin.TimeStamp, svid uint8, usedSlot int) *sbfbin.Block {
	pvt := sbfbin.SlotStatus(sbfbin.PVTStatusUsed) << (2 * usedSlot)
	return &sbfbin.Block{TimeStamp: ts, Params: &sbfbin.ChannelStatus{
		SatInfo: []sbfbin.ChannelSatInfo{{SVID: svid, SVIDFull: uint16(svid), AzimuthRiseSet: 100, Elevation: 30}},
		StateInfo: [][]sbfbin.ChannelStateInfo{{{
			Antenna:   0,
			PVTStatus: pvt,
		}}},
	}}
}

// TestProcessPVTCapture drives an SBF-only PVT capture end to end and checks
// that each EndOfPVT flushes a NavEpochMsg (the required SBF-only path) and
// that position/velocity/time messages are produced.
func TestProcessPVTCapture(t *testing.T) {
	p, c := newTestProcessor()
	feed(t, p, readCapture(t, "pvt-cov.jsonl"))

	if len(c.epochs) < 25 {
		t.Errorf("NavEpoch count = %d, want >= 25 (one per EndOfPVT)", len(c.epochs))
	}
	if len(c.posGeo) < 25 || len(c.posECEF) < 25 {
		t.Errorf("posGeo=%d posECEF=%d, want >= 25 each", len(c.posGeo), len(c.posECEF))
	}
	if len(c.velGeo) < 25 || len(c.velECEF) < 25 {
		t.Errorf("velGeo=%d velECEF=%d, want >= 25 each", len(c.velGeo), len(c.velECEF))
	}
	if len(c.times) == 0 {
		t.Error("no TimeMsg produced")
	}
	ne := c.epochs[0]
	if ne.Tag != Tag {
		t.Errorf("NavEpoch.Tag = %q, want %q", ne.Tag, Tag)
	}
	if ne.FixLevel != gpsprot.FixLevelCode { // Mode==1 standalone in this capture
		t.Errorf("NavEpoch.FixLevel = %v, want %v", ne.FixLevel, gpsprot.FixLevelCode)
	}
	if ne.SolutionDim != gpsprot.SolutionDim3D {
		t.Errorf("NavEpoch.SolutionDim = %v, want 3D", ne.SolutionDim)
	}
	if !ne.NumSVUsed.IsSet() {
		t.Error("NavEpoch.NumSVUsed unset")
	}
	if ne.GNSSUsed.IsZero() {
		t.Error("NavEpoch.GNSSUsed empty (SignalInfo mapping produced nothing)")
	}
}

// TestNavEpochRestartsAfterFlushedDNUKey verifies that consecutive epochs with
// the same DNU timestamp each get an accumulator after protocol-local flushes.
func TestNavEpochRestartsAfterFlushedDNUKey(t *testing.T) {
	p, c := newTestProcessor()
	tRead := time.Unix(0, 0)
	dnu := sbfbin.TimeStamp{TOW: sbfbin.TOWDNU, WNc: sbfbin.WNcDNU}
	for _, ts := range []sbfbin.TimeStamp{{TOW: 1000, WNc: 1}, {TOW: 2000, WNc: 1}, dnu, dnu} {
		p.Dispatch(pvtGeoBlock(ts, sbfbin.TimeSystemGPS, sbfbin.PVTClockDNU), tRead)
		p.Dispatch(&sbfbin.Block{TimeStamp: ts, Params: &sbfbin.EndOfPVT{}}, tRead.Add(time.Millisecond))
		tRead = tRead.Add(time.Second)
	}
	if len(c.epochs) != 4 {
		t.Fatalf("NavEpoch count = %d, want 4", len(c.epochs))
	}
	if c.epochs[2] == c.epochs[3] {
		t.Error("consecutive DNU epochs reused the same NavEpochMsg")
	}
}

// TestReceiverTimeStartsNavEpoch verifies that ReceiverTime replaces a stale
// epoch start and that an epoch containing no PVT block still emits a boundary.
func TestReceiverTimeStartsNavEpoch(t *testing.T) {
	p, c := newTestProcessor()
	tRead := time.Unix(0, 0)
	p.Dispatch(pvtGeoBlock(sbfbin.TimeStamp{TOW: 1000, WNc: 1}, sbfbin.TimeSystemGPS, sbfbin.PVTClockDNU), tRead)
	for i, tow := range []uint32{2000, 3000} {
		m := &sbfbin.ReceiverTime{UTCYear: sbfbin.UTCComponentDNU, DeltaLS: sbfbin.UTCComponentDNU}
		p.Dispatch(&sbfbin.Block{TimeStamp: sbfbin.TimeStamp{TOW: tow, WNc: 1}, Params: m}, tRead.Add(time.Duration(i+1)*time.Second))
	}
	if len(c.times) != 2 {
		t.Fatalf("TimeMsg count = %d, want 2", len(c.times))
	}
	for i, tm := range c.times {
		if tm.ReadDelay != 0 {
			t.Errorf("TimeMsg %d ReadDelay = %v, want 0", i, tm.ReadDelay)
		}
	}
	if len(c.epochs) != 2 {
		t.Fatalf("NavEpoch count = %d, want 2", len(c.epochs))
	}
	if c.epochs[1].Tag != Tag {
		t.Errorf("ReceiverTime-only NavEpoch.Tag = %q, want %q", c.epochs[1].Tag, Tag)
	}
}

// TestProcessStatusCapture drives a status capture and checks
// ChannelStatus-only SVs are emitted.
func TestProcessStatusCapture(t *testing.T) {
	p, c := newTestProcessor()
	feed(t, p, readCapture(t, "status.jsonl"))
	if len(c.sats) == 0 {
		t.Fatal("no SatellitesMsg produced")
	}
	s := c.sats[0]
	if s.Tag != Tag || s.NativeMsgID != "ChannelStatus" || s.UsedValidity != gpsprot.SatelliteUsedSV {
		t.Errorf("SatellitesMsg header = %+v", s)
	}
	if len(s.SVs) == 0 {
		t.Fatal("SatellitesMsg has no SVs")
	}
	for _, sv := range s.SVs {
		if sv.Signals == nil || len(sv.Signals) != 0 {
			t.Errorf("%s signals = %+v, want empty non-nil slice", sv.ID, sv.Signals)
		}
	}
}

func TestSatellitesEmitOnChannelStatusAndConsumeMeasEpoch(t *testing.T) {
	p, c := newTestProcessor()
	tRead := time.Unix(0, 0)
	ts1 := sbfbin.TimeStamp{TOW: 1000, WNc: 1}
	ts2 := sbfbin.TimeStamp{TOW: 2000, WNc: 1}
	p.Dispatch(testMeasBlock(ts1, 1, sbfbin.SigNumGPSL1CA, 200), tRead)
	if len(c.sats) != 0 {
		t.Fatalf("SatellitesMsg count after MeasEpoch = %d, want 0", len(c.sats))
	}
	p.Dispatch(testChannelStatusBlock(ts1, 1, 0), tRead.Add(time.Millisecond))
	if len(c.sats) != 1 {
		t.Fatalf("SatellitesMsg count after ChannelStatus = %d, want 1", len(c.sats))
	}
	s := c.sats[0]
	if s.Tag != Tag || s.NativeMsgID != "ChannelStatus" || s.UsedValidity != gpsprot.SatelliteUsedSignal {
		t.Errorf("SatellitesMsg header = %+v", s)
	}
	if len(s.SVs) != 1 || len(s.SVs[0].Signals) != 1 || s.SVs[0].Signals[0].CN0 != 60 || !s.SVs[0].Signals[0].Used {
		t.Errorf("SatellitesMsg SVs = %+v", s.SVs)
	}
	p.Dispatch(testChannelStatusBlock(ts2, 1, 0), tRead.Add(2*time.Millisecond))
	if len(c.sats) != 2 {
		t.Fatalf("SatellitesMsg count after next ChannelStatus = %d, want 2", len(c.sats))
	}
	s = c.sats[1]
	if s.Tag != Tag || s.NativeMsgID != "ChannelStatus" || s.UsedValidity != gpsprot.SatelliteUsedSV {
		t.Errorf("second SatellitesMsg header = %+v", s)
	}
	if len(s.SVs) != 1 || !s.SVs[0].Used || len(s.SVs[0].Signals) != 0 || s.SVs[0].Signals == nil {
		t.Errorf("second SatellitesMsg SVs = %+v", s.SVs)
	}
}

func TestSatellitesMeasEpochOverwriteWaitsForChannelStatus(t *testing.T) {
	p, c := newTestProcessor()
	tRead := time.Unix(0, 0)
	ts1 := sbfbin.TimeStamp{TOW: 1000, WNc: 1}
	ts2 := sbfbin.TimeStamp{TOW: 2000, WNc: 1}
	p.Dispatch(testMeasBlock(ts1, 1, sbfbin.SigNumGPSL1CA, 200), tRead)
	p.Dispatch(testMeasBlock(ts2, 1, sbfbin.SigNumGPSL1CA, 180), tRead.Add(time.Second))
	if len(c.sats) != 0 {
		t.Fatalf("SatellitesMsg count after MeasEpoch overwrite = %d, want 0", len(c.sats))
	}
	p.Dispatch(testChannelStatusBlock(ts2, 1, 0), tRead.Add(time.Second+time.Millisecond))
	if len(c.sats) != 1 {
		t.Fatalf("SatellitesMsg count after ChannelStatus = %d, want 1", len(c.sats))
	}
	if len(c.sats[0].SVs) != 1 || len(c.sats[0].SVs[0].Signals) != 1 || c.sats[0].SVs[0].Signals[0].CN0 != 55 {
		t.Errorf("SatellitesMsg SVs = %+v", c.sats[0].SVs)
	}
}
