package phcsync

import (
	"log/slog"
	"testing"
	"time"

	"github.com/jclark/satpulse/internal/circbuf"
	"github.com/jclark/satpulse/internal/clocksim"
	"github.com/jclark/satpulse/internal/ptime"
)

type genSampleTestCase struct {
	name string
	// Config modifier: applied to defaultResetConfig(). Nil means use defaults.
	cfgMod func(*ResetConfig)
	// Edge generation parameters
	customIntervals []time.Duration // if non-nil, use these intervals instead of uniform
	numEdges        int
	interval        time.Duration
	phcDrift        float64
	pulseWidth      time.Duration // if non-zero, generate dual edges (EdgesPerPulse=2)
	startWithRising bool          // for dual-edge: true = start with rising, false = start with falling
	// Message generation parameters
	customDelays []time.Duration // if non-nil, use these delays instead of uniform msgDelay
	msgDelay     time.Duration   // uniform delay for all messages
	// Expected outcome
	wantError bool
	wantStats *resetStats // expected stats when test succeeds
}

func TestGenSampleForMessages(t *testing.T) {
	tests := []genSampleTestCase{
		// Single-edge tests
		{
			name:     "HappyPath",
			numEdges: 5,
			interval: time.Second,
			msgDelay: 100 * time.Millisecond,
			wantStats: &resetStats{
				pulseVariation: 0.0,   // perfect 1s intervals
				delay:          0.1,   // 100ms delay
				delayVariation: 0.0,   // uniform delay
			},
		},
		{
			name:      "IntervalTooLarge",
			numEdges:  5,
			interval:  1001 * time.Millisecond,
			msgDelay:  100 * time.Millisecond,
			wantError: true,
		},
		{
			name:            "InconsistentIntervals",
			customIntervals: []time.Duration{0, 999 * time.Millisecond, 1001 * time.Millisecond, 998 * time.Millisecond, 1002 * time.Millisecond},
			msgDelay:        100 * time.Millisecond,
			wantError:       true,
		},
		{
			name:         "DelaySpreadTooLarge",
			numEdges:     5,
			interval:     time.Second,
			customDelays: []time.Duration{50 * time.Millisecond, 100 * time.Millisecond, 300 * time.Millisecond, 100 * time.Millisecond, 50 * time.Millisecond},
			wantError:    true,
		},
		{
			name:      "DelayOutOfRange",
			numEdges:  5,
			interval:  time.Second,
			msgDelay:  500 * time.Millisecond,
			wantError: true,
		},
		{
			name:      "InsufficientData",
			numEdges:  1,
			interval:  time.Second,
			msgDelay:  100 * time.Millisecond,
			wantError: true,
		},
		// Dual-edge tests - various pulse widths, start with rising edge
		{
			name:            "DualEdge_1us_Rising",
			numEdges:        10,
			interval:        time.Second,
			msgDelay:        100 * time.Millisecond,
			pulseWidth:      1 * time.Microsecond,
			startWithRising: true,
			// Note: dual-edge tests add ~20ns noise to pulse width, so pulseVariation won't be exactly 0
		},
		{
			name:            "DualEdge_1ms_Rising",
			numEdges:        10,
			interval:        time.Second,
			msgDelay:        100 * time.Millisecond,
			pulseWidth:      1 * time.Millisecond,
			startWithRising: true,
		},
		{
			name:            "DualEdge_10ms_Rising",
			numEdges:        10,
			interval:        time.Second,
			msgDelay:        100 * time.Millisecond,
			pulseWidth:      10 * time.Millisecond,
			startWithRising: true,
		},
		{
			name:            "DualEdge_100ms_Rising",
			numEdges:        10,
			interval:        time.Second,
			msgDelay:        100 * time.Millisecond,
			pulseWidth:      100 * time.Millisecond,
			startWithRising: true,
		},
		{
			name:            "DualEdge_200ms_Rising",
			numEdges:        10,
			interval:        time.Second,
			msgDelay:        100 * time.Millisecond,
			pulseWidth:      200 * time.Millisecond,
			startWithRising: true,
		},
		{
			name:            "DualEdge_400ms_Rising",
			numEdges:        10,
			interval:        time.Second,
			msgDelay:        100 * time.Millisecond,
			pulseWidth:      400 * time.Millisecond,
			startWithRising: true,
		},
		// Dual-edge tests - start with falling edge
		{
			name:            "DualEdge_10ms_Falling",
			numEdges:        10,
			interval:        time.Second,
			msgDelay:        100 * time.Millisecond,
			pulseWidth:      10 * time.Millisecond,
			startWithRising: false,
		},
		{
			name:            "DualEdge_100ms_Falling",
			numEdges:        10,
			interval:        time.Second,
			msgDelay:        100 * time.Millisecond,
			pulseWidth:      100 * time.Millisecond,
			startWithRising: false,
		},
		{
			name:            "DualEdge_400ms_Falling",
			numEdges:        10,
			interval:        time.Second,
			msgDelay:        100 * time.Millisecond,
			pulseWidth:      400 * time.Millisecond,
			startWithRising: false,
			wantError:       true, // Falling edge first with large pulse width causes delay check to fail
		},
		// Dual-edge test near 50% duty cycle (ambiguous)
		{
			name:       "DualEdge_500ms_Ambiguous",
			numEdges:   10,
			interval:   time.Second,
			msgDelay:   100 * time.Millisecond,
			pulseWidth: 500 * time.Millisecond,
			cfgMod: func(c *ResetConfig) {
				c.PulseWidthDetectLimit = 0.45 // 500ms is exactly 50%, should fail with default detect limit
			},
			startWithRising: true,
			wantError:       true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := defaultResetConfig()
			if tc.cfgMod != nil {
				tc.cfgMod(&cfg)
			}

			var gen *resetSampleGenerator
			var tRead []time.Time
			var lastSec ptime.Time

			if tc.customIntervals != nil {
				gen = setupGeneratorCustomIntervals(cfg, tc.customIntervals)
				baseTime := time.Now()
				n := len(tc.customIntervals)
				tRead = make([]time.Time, n)
				for i := 0; i < n; i++ {
					tRead[i] = baseTime.Add(time.Duration(i)*time.Second + tc.msgDelay)
				}
				lastSec = ptime.Time(0).Add(time.Duration(n-1) * time.Second)
			} else {
				gen = setupGenerator(cfg, tc.numEdges, tc.interval, tc.msgDelay, tc.phcDrift, tc.pulseWidth, tc.startWithRising)
				baseTime := time.Now()

				// Calculate number of pulses (messages)
				numPulses := tc.numEdges
				if tc.pulseWidth > 0 {
					// Dual-edge mode: 2 edges per pulse
					numPulses = tc.numEdges / 2
					if tc.numEdges%2 == 1 {
						numPulses++
					}
				}

				if tc.customDelays != nil {
					tRead = make([]time.Time, len(tc.customDelays))
					for i := 0; i < len(tc.customDelays); i++ {
						tRead[i] = baseTime.Add(time.Duration(i)*tc.interval + tc.customDelays[i])
					}
				} else {
					tRead = make([]time.Time, numPulses)
					for i := 0; i < numPulses; i++ {
						tRead[i] = baseTime.Add(time.Duration(i)*tc.interval + tc.msgDelay)
					}
				}
				lastSec = ptime.Time(0).Add(time.Duration(numPulses-1) * time.Second)
			}

			sample, stats, err := gen.genSampleForMessages(lastSec, tRead)

			if tc.wantError {
				if err == nil {
					t.Errorf("expected error, got nil")
				}
				if sample != nil {
					t.Errorf("expected nil sample on error, got %v", sample)
				}
				if stats != nil {
					t.Errorf("expected nil stats on error, got %v", stats)
				}
			} else {
				if err != nil {
					t.Fatalf("expected no error, got: %v", err)
				}
				if sample == nil {
					t.Fatal("expected sample, got nil")
				}
				if stats == nil {
					t.Fatal("expected stats, got nil")
				}
				if sample.Kind != SampleOK {
					t.Errorf("expected SampleOK, got %v", sample.Kind)
				}
				if sample.Ref != lastSec {
					t.Errorf("expected Ref=%v, got %v", lastSec, sample.Ref)
				}
				if tc.wantStats != nil {
					tolerance := 0.001
					if abs(stats.pulseVariation-tc.wantStats.pulseVariation) > tolerance {
						t.Errorf("pulseVariation: want %v, got %v", tc.wantStats.pulseVariation, stats.pulseVariation)
					}
					if abs(stats.delay-tc.wantStats.delay) > tolerance {
						t.Errorf("delay: want %v, got %v", tc.wantStats.delay, stats.delay)
					}
					if abs(stats.delayVariation-tc.wantStats.delayVariation) > tolerance {
						t.Errorf("delayVariation: want %v, got %v", tc.wantStats.delayVariation, stats.delayVariation)
					}
				}
			}
		})
	}
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

// setupGenerator creates an resetSampleGenerator with synthetic edge data.
// It creates numEdges edges with the specified interval and message delay.
// phcDrift specifies how much the PHC drifts from real time per second (e.g., 100e-9 for 100ns/s).
// If pulseWidth > 0, generates dual edges (rising and falling) per pulse.
// startWithRising determines whether first edge is rising (true) or falling (false).
func setupGenerator(cfg ResetConfig, numEdges int, interval, msgDelay time.Duration, phcDrift float64, pulseWidth time.Duration, startWithRising bool) *resetSampleGenerator {
	edgesPerPulse := 1
	if pulseWidth > 0 {
		edgesPerPulse = 2
	}

	// Buffer size needs to accommodate all edges
	bufSize := numEdges
	if bufSize < cfg.PulseWindow {
		bufSize = cfg.PulseWindow
	}

	pt := PulseType{EdgesPerPulse: edgesPerPulse, PulseWidth: pulseWidth}
	gen := &resetSampleGenerator{
		timeMsgBuffer: nil, // not needed when testing genSampleForMessages directly
		edgeBuf:       circbuf.New[PulseEdge](bufSize),
		cfg:           cfg,
		lg:            slog.Default(),
		pt:            pt,
		maxFreq:       500.0, // typical max frequency adjustment
		freq:          0.0,
	}

	// Generate edges
	baseTime := time.Now()
	basePHC := ptime.Time(0)

	if edgesPerPulse == 1 {
		// Single-edge mode
		for i := 0; i < numEdges; i++ {
			realTime := baseTime.Add(time.Duration(i) * interval)
			phcTime := basePHC.Add(time.Duration(float64(i*int(interval)) * (1.0 + phcDrift)))

			readDelay := 1 * time.Millisecond
			tRead := realTime.Add(readDelay)
			tReadPHC := phcTime.Add(readDelay)

			gen.storeEdge(PulseEdge{
				Timestamp: ptime.ClockTime{T: phcTime, Era: 0},
				TRead: ptime.Sample{
					Clock: ptime.ClockTime{T: tReadPHC, Era: 0},
					Sys:   tRead,
				},
			}, uint64(i))
		}
	} else {
		// Dual-edge mode: generate both rising and falling edges
		numPulses := numEdges / 2
		if numEdges%2 == 1 {
			numPulses++
		}

		// Add white noise to pulse width for realism (~20ns stddev)
		noiseGen := clocksim.WhiteNoisePPS(20*time.Nanosecond, 12345)

		for i := 0; i < numPulses; i++ {
			// Calculate pulse start time (rising edge)
			risingRealTime := baseTime.Add(time.Duration(i) * interval)
			risingPHCTime := basePHC.Add(time.Duration(float64(i*int(interval)) * (1.0 + phcDrift)))

			// Calculate falling edge time with noise added to pulse width
			// Noise is added in real time domain, then scaled by PHC drift
			noiseSeconds := noiseGen(float64(i))
			noisyPulseWidth := pulseWidth + time.Duration(noiseSeconds*1e9)
			fallingRealTime := risingRealTime.Add(noisyPulseWidth)
			fallingPHCTime := risingPHCTime.Add(time.Duration(float64(noisyPulseWidth) * (1.0 + phcDrift)))

			readDelay := 1 * time.Millisecond

			// Add edges in the requested order
			edges := []struct {
				phcTime  ptime.Time
				realTime time.Time
			}{
				{risingPHCTime, risingRealTime},
				{fallingPHCTime, fallingRealTime},
			}

			if !startWithRising {
				edges[0], edges[1] = edges[1], edges[0]
			}

			for j, edge := range edges {
				edgeIndex := i*2 + j
				if edgeIndex >= numEdges {
					break
				}
				gen.storeEdge(PulseEdge{
					Timestamp: ptime.ClockTime{T: edge.phcTime, Era: 0},
					TRead: ptime.Sample{
						Clock: ptime.ClockTime{T: edge.phcTime.Add(readDelay), Era: 0},
						Sys:   edge.realTime.Add(readDelay),
					},
				}, uint64(edgeIndex))
			}
		}
	}

	return gen
}

// setupGeneratorCustomIntervals creates an resetSampleGenerator with custom intervals.
// intervals[0] should be 0, intervals[i] is the duration from edge i-1 to edge i.
func setupGeneratorCustomIntervals(cfg ResetConfig, intervals []time.Duration) *resetSampleGenerator {
	pt := PulseType{EdgesPerPulse: 1, PulseWidth: 0}
	gen := &resetSampleGenerator{
		timeMsgBuffer: nil,
		edgeBuf:       circbuf.New[PulseEdge](cfg.PulseWindow),
		cfg:           cfg,
		lg:            slog.Default(),
		pt:            pt,
		maxFreq:       500.0,
		freq:          0.0,
	}

	baseTime := time.Now()
	basePHC := ptime.Time(0)
	phcTime := basePHC
	realTime := baseTime

	for i := 0; i < len(intervals); i++ {
		if i > 0 {
			phcTime = phcTime.Add(intervals[i])
			realTime = realTime.Add(intervals[i])
		}
		readDelay := 1 * time.Millisecond
		gen.storeEdge(PulseEdge{
			Timestamp: ptime.ClockTime{T: phcTime, Era: 0},
			TRead: ptime.Sample{
				Clock: ptime.ClockTime{T: phcTime.Add(readDelay), Era: 0},
				Sys:   realTime.Add(readDelay),
			},
		}, uint64(i))
	}

	return gen
}

// TestPulseTimestamps verifies that pulseTimestamps extracts timestamps in correct order
func TestPulseTimestamps(t *testing.T) {
	cfg := defaultResetConfig()
	gen := setupGenerator(cfg, 3, time.Second, 100*time.Millisecond, 0, 0, true)

	timestamps := gen.pulseTimestamps()
	if len(timestamps) != 3 {
		t.Fatalf("expected 3 timestamps, got %d", len(timestamps))
	}

	// Timestamps should be in ascending order (oldest to newest)
	for i := 1; i < len(timestamps); i++ {
		if timestamps[i] <= timestamps[i-1] {
			t.Errorf("timestamps not in ascending order: timestamps[%d]=%v not after timestamps[%d]=%v",
				i, timestamps[i], i-1, timestamps[i-1])
		}
	}
}

// TestPulseIntervals verifies interval calculation
func TestPulseIntervals(t *testing.T) {
	gen := &resetSampleGenerator{lg: slog.Default()}

	timestamps := []ptime.Time{
		ptime.Time(0).Add(0),
		ptime.Time(0).Add(1 * time.Second),
		ptime.Time(0).Add(2 * time.Second),
	}

	intervals := gen.pulseIntervals(timestamps)
	if len(intervals) != 2 {
		t.Fatalf("expected 2 intervals, got %d", len(intervals))
	}

	for i, interval := range intervals {
		if interval != time.Second {
			t.Errorf("intervals[%d] = %v, want %v", i, interval, time.Second)
		}
	}
}

// TestLastEdgeIndexUpdated verifies that genSampleForMessages updates lastEdgeIndex
// to match the selected edge list's parity after filtering in dual-edge mode.
func TestLastEdgeIndexUpdated(t *testing.T) {
	tests := []struct {
		name            string
		pulseWidth      time.Duration
		startWithRising bool
		wantParityEven  bool
	}{
		{
			name:            "100ms pulse rising first",
			pulseWidth:      100 * time.Millisecond,
			startWithRising: true,
			wantParityEven:  true, // rising edges have even indices
		},
		{
			name:            "200ms pulse rising first",
			pulseWidth:      200 * time.Millisecond,
			startWithRising: true,
			wantParityEven:  true, // rising edges have even indices
		},
		{
			name:            "100ms pulse falling first",
			pulseWidth:      100 * time.Millisecond,
			startWithRising: false,
			wantParityEven:  true, // falling edges (leading) have even indices when falling is first
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := defaultResetConfig()
			numEdges := 10
			interval := time.Second
			msgDelay := 100 * time.Millisecond

			gen := setupGenerator(cfg, numEdges, interval, msgDelay, 0, tc.pulseWidth, tc.startWithRising)

			// Before calling genSampleForMessages, lastEdgeIndex should be 9 (last appended edge)
			if gen.lastEdgeIndex != 9 {
				t.Fatalf("expected lastEdgeIndex=9 before genSampleForMessages, got %d", gen.lastEdgeIndex)
			}

			baseTime := time.Now()
			numPulses := numEdges / 2
			tRead := make([]time.Time, numPulses)
			for i := 0; i < numPulses; i++ {
				tRead[i] = baseTime.Add(time.Duration(i)*interval + msgDelay)
			}
			lastSec := ptime.Time(0).Add(time.Duration(numPulses-1) * time.Second)

			sample, _, err := gen.genSampleForMessages(lastSec, tRead)
			if err != nil {
				t.Fatalf("genSampleForMessages failed: %v", err)
			}
			if sample == nil {
				t.Fatal("expected sample, got nil")
			}

			// After genSampleForMessages, lastEdgeIndex should match selected edge list's parity
			isEven := gen.lastEdgeIndex%2 == 0
			if isEven != tc.wantParityEven {
				t.Errorf("lastEdgeIndex=%d has wrong parity: got even=%v, want even=%v",
					gen.lastEdgeIndex, isEven, tc.wantParityEven)
			}
		})
	}
}
