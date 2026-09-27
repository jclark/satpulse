package rnxnov

import (
	"bufio"
	"encoding/hex"
	"encoding/json"
	"math"
	"os"
	"reflect"
	"testing"

	"github.com/jclark/satpulse/gps/lib/novmsg"
	"github.com/jclark/satpulse/gps/lib/opt"
	"github.com/jclark/satpulse/gps/lib/rinex"
)

type testSink struct {
	obs []rinex.SignalObservation
}

func (s *testSink) Metadata(rinex.Metadata) error {
	return nil
}

func (s *testSink) Observation(obs rinex.SignalObservation) error {
	s.obs = append(s.obs, obs)
	return nil
}

func (s *testSink) Flush() error {
	return nil
}

func TestConvertRange(t *testing.T) {
	t0 := rinex.TimeFromGPSWeekMillis(2438, 1000)
	tests := []struct {
		name           string
		mapping        novmsg.RangeMapping
		obs            []novmsg.RangeObs
		expect         []rinex.SignalObservation
		expectUnmapped int
	}{
		{
			name:    "GPS L1C/A",
			mapping: novmsg.RangeMappingOEM7,
			obs: []novmsg.RangeObs{{
				PRN: 7, PSR: 23956830.53, ADR: -125893980.172, Dopp: -12.5, CN0: 45.3, LockTime: 100,
				Status: status(novmsg.SatSystemGPS, 0, true, true, true),
			}},
			expect: []rinex.SignalObservation{{
				T: t0, Sat: "G07", Sig: "1C",
				SignalValues: rinex.SignalValues{
					PR: opt.Make(23956830.53), CP: opt.Make(125893980.172), Do: opt.Make(-12.5), CN0: opt.Make[float32](45.3),
				},
			}},
		},
		{
			name:    "validity bits",
			mapping: novmsg.RangeMappingOEM7,
			obs: []novmsg.RangeObs{
				{PRN: 7, PSR: 1, ADR: -2, Dopp: 3, LockTime: 100, Status: status(novmsg.SatSystemGPS, 0, false, true, true)},
				{PRN: 8, PSR: 1, ADR: -2, Dopp: 3, LockTime: 100, Status: status(novmsg.SatSystemGPS, 0, true, false, true)},
				{PRN: 9, PSR: math.NaN(), ADR: math.Inf(1), Dopp: float32(math.NaN()), LockTime: 100, Status: status(novmsg.SatSystemGPS, 0, true, true, true)},
			},
			expect: []rinex.SignalObservation{
				{T: t0, Sat: "G07", Sig: "1C", SignalValues: rinex.SignalValues{CP: opt.Make(2.0), Do: opt.Make(3.0)}},
				{T: t0, Sat: "G08", Sig: "1C", SignalValues: rinex.SignalValues{PR: opt.Make(1.0), Do: opt.Make(3.0)}},
			},
		},
		{
			name:    "half cycle",
			mapping: novmsg.RangeMappingOEM7,
			obs: []novmsg.RangeObs{
				{PRN: 7, PSR: 1, ADR: -2, LockTime: 100, Status: status(novmsg.SatSystemGPS, 0, true, true, false)},
				{PRN: 8, PSR: 1, LockTime: 100, Status: status(novmsg.SatSystemGPS, 0, true, false, false)},
			},
			expect: []rinex.SignalObservation{
				{T: t0, Sat: "G07", Sig: "1C", SignalValues: rinex.SignalValues{PR: opt.Make(1.0), CP: opt.Make(2.0), Do: opt.Make(0.0), HC: true}},
				{T: t0, Sat: "G08", Sig: "1C", SignalValues: rinex.SignalValues{PR: opt.Make(1.0), Do: opt.Make(0.0)}},
			},
		},
		{
			name:    "GLONASS frequency channel",
			mapping: novmsg.RangeMappingOEM7,
			obs: []novmsg.RangeObs{
				{PRN: 38, GloFreq: 0, PSR: 1, Status: status(novmsg.SatSystemGLONASS, 0, true, false, true)},
				{PRN: 39, GloFreq: 13, PSR: 1, Status: status(novmsg.SatSystemGLONASS, 5, true, false, true)},
				{PRN: 40, GloFreq: 14, PSR: 1, Status: status(novmsg.SatSystemGLONASS, 0, true, false, true)},
			},
			expect: []rinex.SignalObservation{
				{T: t0, Sat: "R01", Sig: "1C", SignalValues: rinex.SignalValues{Frq: opt.Make[int8](-7), PR: opt.Make(1.0), Do: opt.Make(0.0)}},
				{T: t0, Sat: "R02", Sig: "2P", SignalValues: rinex.SignalValues{Frq: opt.Make[int8](6), PR: opt.Make(1.0), Do: opt.Make(0.0)}},
				{T: t0, Sat: "R03", Sig: "1C", SignalValues: rinex.SignalValues{PR: opt.Make(1.0), Do: opt.Make(0.0)}},
			},
		},
		{
			name:    "SinoGNSS mapping",
			mapping: novmsg.RangeMappingSinoGNSS,
			obs: []novmsg.RangeObs{
				{PRN: 141, PSR: 1, Status: status(novmsg.SatSystemBeiDou, 19, true, false, true)},
				{PRN: 132, PSR: 1, Status: status(novmsg.SatSystemQZSS, 0, true, false, true)},
				{PRN: 7, PSR: 1, Status: status(novmsg.SatSystemGPS, 2, true, false, true)},
			},
			expect: []rinex.SignalObservation{
				{T: t0, Sat: "C01", Sig: "7D", SignalValues: rinex.SignalValues{PR: opt.Make(1.0), Do: opt.Make(0.0)}},
				{T: t0, Sat: "J02", Sig: "1C", SignalValues: rinex.SignalValues{PR: opt.Make(1.0), Do: opt.Make(0.0)}},
				{T: t0, Sat: "G07", Sig: "5Q", SignalValues: rinex.SignalValues{PR: opt.Make(1.0), Do: opt.Make(0.0)}},
			},
		},
		{
			name:    "SinoGNSS log with OEM7 mapping",
			mapping: novmsg.RangeMappingOEM7,
			obs: []novmsg.RangeObs{
				{PRN: 141, PSR: 1, Status: status(novmsg.SatSystemBeiDou, 19, true, false, true)},
				{PRN: 132, PSR: 1, Status: status(novmsg.SatSystemQZSS, 0, true, false, true)},
				{PRN: 7, PSR: 1, Status: status(novmsg.SatSystemGPS, 2, true, false, true)},
			},
			expectUnmapped: 3,
		},
		{
			name:    "no values",
			mapping: novmsg.RangeMappingOEM7,
			obs: []novmsg.RangeObs{
				{PRN: 7, Dopp: float32(math.NaN()), Status: status(novmsg.SatSystemGPS, 0, false, false, false)},
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := &testSink{}
			c := New(s, tc.mapping)
			ok, err := c.ConvertRange(hdr(1000), rangeLog(tc.obs...))
			if err != nil {
				t.Fatalf("ConvertRange: %v", err)
			}
			if ok != (len(tc.expect) > 0) {
				t.Errorf("ok = %v", ok)
			}
			if !reflect.DeepEqual(s.obs, tc.expect) {
				t.Errorf("got  %+v\nwant %+v", s.obs, tc.expect)
			}
			if c.Unmapped() != tc.expectUnmapped {
				t.Errorf("Unmapped() = %d, want %d", c.Unmapped(), tc.expectUnmapped)
			}
		})
	}
}

func TestConvertRangeArcs(t *testing.T) {
	gps := status(novmsg.SatSystemGPS, 0, true, true, true)
	noPhase := status(novmsg.SatSystemGPS, 0, true, false, true)
	epochs := []struct {
		ms     uint32
		lock   float32
		st     novmsg.ChTrStatus
		expect uint32
	}{
		{1000, 100, gps, 0},
		{2000, 101, gps, 0},
		{3000, 101.96, gps, 0},  // within tolerance
		{4000, 102.9, gps, 1},   // grew by less than the elapsed time
		{5000, 0, noPhase, 2},   // reset
		{6000, 1, noPhase, 3},   // still no phase
		{7000, 2, gps, 4},       // phase
		{8000, 3, gps, 4},       // continuing
		{10000, 3.5, gps, 5},    // gap
		{11000, 4.5, gps, 5},    // continuing
		{12000, 0, gps, 6},      // reset with phase
		{13000, 0.9999, gps, 6}, // within tolerance
		{14000, 1.94, gps, 7},   // outside tolerance
		{15000, 2.94, noPhase, 7},
	}
	s := &testSink{}
	c := New(s, novmsg.RangeMappingOEM7)
	var expect []uint32
	for _, e := range epochs {
		if _, err := c.ConvertRange(hdr(e.ms), rangeLog(novmsg.RangeObs{PRN: 7, PSR: 1, ADR: -2, LockTime: e.lock, Status: e.st})); err != nil {
			t.Fatalf("ConvertRange: %v", err)
		}
		expect = append(expect, e.expect)
	}
	var got []uint32
	for _, obs := range s.obs {
		got = append(got, obs.Arc)
	}
	if !reflect.DeepEqual(got, expect) {
		t.Errorf("got  %v\nwant %v", got, expect)
	}
}

// TestConvertRangeK901 converts the binary RANGE of the K901 binary and
// ASCII pair, and checks the first record of each system.
func TestConvertRangeK901(t *testing.T) {
	h, m := readRange(t, "../../testdata/packets/sinognss/K901/raw-obs-ascii.jsonl")
	s := &testSink{}
	c := New(s, novmsg.RangeMappingSinoGNSS)
	if _, err := c.ConvertRange(h, m); err != nil {
		t.Fatalf("ConvertRange: %v", err)
	}
	if len(s.obs) != len(m.Obs) || c.Unmapped() != 0 {
		t.Fatalf("%d observations and %d unmapped from %d records", len(s.obs), c.Unmapped(), len(m.Obs))
	}
	var got []rinex.SignalObservation
	seen := make(map[byte]bool)
	for _, obs := range s.obs {
		if !seen[obs.Sat[0]] {
			seen[obs.Sat[0]] = true
			got = append(got, obs)
		}
	}
	t0 := rinex.TimeFromGPSWeekMillis(2438, 12523350)
	expect := []rinex.SignalObservation{
		{T: t0, Sat: "G18", Sig: "1C", SignalValues: rinex.SignalValues{PR: opt.Make(2.1714497251271527e+07), CP: opt.Make(1.1411040483098936e+08), Do: opt.Make(-2946.637451171875), CN0: opt.Make[float32](45.3)}},
		{T: t0, Sat: "J02", Sig: "1C", SignalValues: rinex.SignalValues{PR: opt.Make(3.7707469217350915e+07), CP: opt.Make(1.9815409072057247e+08), Do: opt.Make(-256.7090759277344), CN0: opt.Make[float32](38.2)}},
		{T: t0, Sat: "C03", Sig: "2I", SignalValues: rinex.SignalValues{PR: opt.Make(3.604564827219655e+07), CP: opt.Make(1.876990088945346e+08), Do: opt.Make(-4.79349422454834), CN0: opt.Make[float32](48.6)}},
		{T: t0, Sat: "R21", Sig: "1C", SignalValues: rinex.SignalValues{Frq: opt.Make[int8](4), PR: opt.Make(2.157896692108388e+07), CP: opt.Make(1.1547335366458702e+08), Do: opt.Make(-3609.231201171875), CN0: opt.Make[float32](49.1)}},
		{T: t0, Sat: "E12", Sig: "1C", SignalValues: rinex.SignalValues{PR: opt.Make(2.3741718970487997e+07), CP: opt.Make(1.2476353615075874e+08), Do: opt.Make(-1444.1932373046875), CN0: opt.Make[float32](44.4)}},
		{T: t0, Sat: "S37", Sig: "1C", SignalValues: rinex.SignalValues{PR: opt.Make(3.67367066843908e+07), CP: opt.Make(1.9305266318938446e+08), Do: opt.Make(-0.26254475116729736), CN0: opt.Make[float32](41.4)}},
		{T: t0, Sat: "I02", Sig: "5A", SignalValues: rinex.SignalValues{PR: opt.Make(3.917318729415533e+07), CP: opt.Make(1.5372392759786558e+08), Do: opt.Make(-241.2716064453125), CN0: opt.Make[float32](46.1)}},
	}
	if !reflect.DeepEqual(got, expect) {
		t.Errorf("got  %+v\nwant %+v", got, expect)
	}
}

// readRange returns the first binary RANGE in a packet log.
func readRange(t *testing.T, path string) (*novmsg.MsgHdr[novmsg.Port], *novmsg.Range) {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		var r struct {
			Msg string `json:"msg"`
			Bin string `json:"bin"`
		}
		if json.Unmarshal(sc.Bytes(), &r) != nil || r.Msg != "RANGE" || r.Bin == "" {
			continue
		}
		b, err := hex.DecodeString(r.Bin)
		if err != nil {
			t.Fatal(err)
		}
		msg, err := novmsg.ParseBinMsg(b)
		if err != nil {
			t.Fatal(err)
		}
		return &msg.Hdr, msg.Body.(*novmsg.Range)
	}
	t.Fatalf("no binary RANGE in %s", path)
	return nil, nil
}

func hdr(ms uint32) *novmsg.MsgHdr[novmsg.Port] {
	return &novmsg.MsgHdr[novmsg.Port]{CommonHdr: novmsg.CommonHdr{Week: 2438, MillisecondsOfWeek: novmsg.GPSec(ms)}}
}

func rangeLog(obs ...novmsg.RangeObs) *novmsg.Range {
	return &novmsg.Range{RangeFixed: novmsg.RangeFixed{NumObs: uint32(len(obs))}, Obs: obs}
}

func status(sys novmsg.SatSystem, sigType uint8, code, phase, parity bool) novmsg.ChTrStatus {
	s := uint32(sys)<<16 | uint32(sigType)<<21
	if phase {
		s |= 1 << 10
	}
	if parity {
		s |= 1 << 11
	}
	if code {
		s |= 1 << 12
	}
	return novmsg.ChTrStatus(s)
}
