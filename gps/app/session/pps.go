package session

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"sync"
	"time"

	"github.com/jclark/satpulse/gps/app/gpsio"
	"github.com/jclark/satpulse/gps/app/serialpps"
)

// PPSConfig is the serial PPS detection configuration passed from the
// frontend to start edge detection.
type PPSConfig struct {
	Pin            string  `json:"pin"`            // cts, dcd, dsr, or ri
	InvertPolarity bool    `json:"invertPolarity"` // pulse asserts the pin instead of deasserting it
	Method         string  `json:"method"`         // poll, wait, or kernel; empty for automatic
	PollPreWarm    float64 `json:"pollPreWarm"`    // seconds; poll method only
}

// PPSEvent is the gps:pps event payload and the PPSState snapshot.
type PPSEvent struct {
	State  string     `json:"state"` // "stopped", "running", "failed"
	Config *PPSConfig `json:"config,omitempty"`
	Method string     `json:"method,omitempty"` // detection method in use; "replay" when simulated
	Error  string     `json:"error,omitempty"`
	Sim    bool       `json:"sim,omitzero"` // a replay recording is configured
}

// EventName implements Event.
func (PPSEvent) EventName() EventName { return EventPPS }

// PulseEdgeEvent is the gps:pulseEdge event payload: one detected candidate
// edge, carrying the same information as satpulsetool serial -p -j.
type PulseEdgeEvent struct {
	T           string  `json:"t"`
	Uncertainty float64 `json:"uncertainty,omitzero"`
	Settling    bool    `json:"settling,omitzero"`
}

// EventName implements Event.
func (PulseEdgeEvent) EventName() EventName { return EventPulseEdge }

// StartPPS starts serial PPS edge detection with the given configuration
// on the session's serial connection. It requires StateConnected and
// returns once detection is set up; edges arrive as gps:pulseEdge events
// and state changes as gps:pps events. At most one detection runs at a
// time: starting while one is running restarts it. When Options.PPSReplay
// is set, a recording is replayed instead and no connection is needed.
func (s *Session) StartPPS(cfg PPSConfig) error {
	w, spCfg, err := cfg.parse()
	if err != nil {
		return err
	}
	s.mu.Lock()
	if s.ppsStopping {
		s.mu.Unlock()
		return fmt.Errorf("PPS stopping")
	}
	s.stopPPSLocked()
	replay := s.opts.PPSReplay
	parent := context.Background()
	var sr serialpps.StateReader
	if replay == "" {
		if s.state != StateConnected || s.runCtx == nil || s.runCtx.Err() != nil {
			err := s.stateErrLocked()
			s.mu.Unlock()
			return err
		}
		var ok bool
		if sr, ok = s.conn.(serialpps.StateReader); !ok {
			s.mu.Unlock()
			return fmt.Errorf("PPS detection requires a serial connection")
		}
		parent = s.runCtx
	}
	ppsCtx, cancel := context.WithCancel(parent)
	s.ppsCancel = cancel
	wg := &sync.WaitGroup{}
	s.ppsWg = wg
	method := ""
	if replay != "" {
		method = "replay"
	}
	running := PPSEvent{State: "running", Config: &cfg, Method: method, Sim: replay != ""}
	s.setPPSStateLocked(running)
	ceCh := make(chan serialpps.CandidateEdge, 1)
	selected := func(m gpsio.PPSMethod) {
		s.mu.Lock()
		ev := s.ppsState
		if ev.State != "running" {
			s.mu.Unlock()
			return
		}
		ev.Method = m.String()
		s.setPPSStateLocked(ev)
		s.mu.Unlock()
		s.emit(ev)
	}
	wg.Go(func() {
		var err error
		if replay != "" {
			err = replayEdges(ppsCtx, replay, ceCh)
		} else {
			err = serialpps.Detect(ppsCtx, s.lg, sr, w, spCfg, ceCh, nil, selected)
		}
		if ppsCtx.Err() != nil || err == nil || errors.Is(err, context.Canceled) {
			s.emitPPSState(PPSEvent{State: "stopped", Sim: replay != ""})
			return
		}
		s.emitPPSState(PPSEvent{State: "failed", Config: &cfg, Method: method, Error: err.Error(), Sim: replay != ""})
	})
	wg.Go(func() {
		for {
			select {
			case <-ppsCtx.Done():
				return
			case ce := <-ceCh:
				s.emit(PulseEdgeEvent{
					T:           ce.Timestamp.UTC().Round(time.Microsecond).Format("2006-01-02T15:04:05.000000Z"),
					Uncertainty: ce.Uncertainty.Seconds(),
					Settling:    !ce.Settled,
				})
			}
		}
	})
	if replay == "" {
		s.connWg.Go(func() { wg.Wait() })
	}
	s.mu.Unlock()
	s.emit(running)
	return nil
}

// parse validates cfg and translates it into the serialpps wiring and
// detection configuration.
func (cfg PPSConfig) parse() (serialpps.Wiring, serialpps.Config, error) {
	var w serialpps.Wiring
	switch cfg.Pin {
	case "cts":
		w.Pin = gpsio.ModemCTS
	case "dcd":
		w.Pin = gpsio.ModemDCD
	case "dsr":
		w.Pin = gpsio.ModemDSR
	case "ri":
		w.Pin = gpsio.ModemRI
	default:
		return w, serialpps.Config{}, fmt.Errorf("pin must be one of cts, dcd, dsr, or ri")
	}
	if cfg.InvertPolarity {
		w.Polarity = serialpps.PolarityAssert
	}
	spCfg := serialpps.Config{PollPreWarm: cfg.PollPreWarm}
	if cfg.Method != "" {
		var err error
		if spCfg.Method, err = gpsio.ParsePPSMethod(cfg.Method); err != nil {
			return w, spCfg, fmt.Errorf("method must be poll, wait, or kernel, or empty for automatic")
		}
	}
	if math.IsNaN(cfg.PollPreWarm) || cfg.PollPreWarm < 0 || cfg.PollPreWarm >= 1 {
		return w, spCfg, fmt.Errorf("poll pre-warm must be at least 0 and less than 1")
	}
	return w, spCfg, nil
}

// StopPPS stops PPS edge detection and waits for its goroutines to exit;
// it is a no-op when none is running. It returns an error only when a
// stop is already in progress.
func (s *Session) StopPPS() error {
	s.mu.Lock()
	if s.ppsStopping {
		s.mu.Unlock()
		return fmt.Errorf("PPS stopping")
	}
	s.stopPPSLocked()
	s.mu.Unlock()
	return nil
}

// stopPPSLocked cancels the detection context and waits for the PPS
// goroutines to exit. Must be called with s.mu held; temporarily
// releases s.mu while waiting.
func (s *Session) stopPPSLocked() {
	if s.ppsCancel == nil {
		s.setPPSStateLocked(PPSEvent{State: "stopped", Sim: s.opts.PPSReplay != ""})
		return
	}
	s.ppsCancel()
	s.ppsCancel = nil
	wg := s.ppsWg
	s.ppsWg = nil
	s.ppsStopping = true
	s.mu.Unlock()
	wg.Wait()
	s.mu.Lock()
	s.ppsStopping = false
}

// PPSState returns the last known PPS detection state.
func (s *Session) PPSState() PPSEvent {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.ppsState
}

func (s *Session) setPPSStateLocked(ev PPSEvent) {
	s.ppsState = ev
}

func (s *Session) emitPPSState(ev PPSEvent) {
	s.mu.Lock()
	s.setPPSStateLocked(ev)
	s.mu.Unlock()
	s.emit(ev)
}

// replayEdge is one recorded edge from a satpulsetool serial -p -j run.
type replayEdge struct {
	t           time.Time
	uncertainty time.Duration
	settling    bool
}

// replayEdges replays a recorded edge sequence in real time. The recorded
// timestamps are rebased onto the current clock by a whole number of
// seconds, preserving each edge's fractional-second offset and the
// inter-edge pacing; at the end of the recording the sequence loops.
func replayEdges(ctx context.Context, path string, ceCh chan<- serialpps.CandidateEdge) error {
	edges, err := parseEdgeRecording(path)
	if err != nil {
		return err
	}
	shift := time.Since(edges[0].t).Truncate(time.Second) + time.Second
	inc := edges[len(edges)-1].t.Sub(edges[0].t).Truncate(time.Second) + time.Second
	for {
		for _, e := range edges {
			target := e.t.Add(shift)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Until(target)):
			}
			ce := serialpps.CandidateEdge{
				Edge:        serialpps.Edge{Timestamp: target, TRead: time.Now()},
				Uncertainty: e.uncertainty,
				Settled:     !e.settling,
			}
			select {
			case ceCh <- ce:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		shift += inc
	}
}

// parseEdgeRecording reads the JSONL edge lines from a serial -p -j
// recording, skipping lines that are not edge records (such as
// interleaved log output).
func parseEdgeRecording(path string) ([]replayEdge, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var edges []replayEdge
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var rec struct {
			T           string  `json:"t"`
			Uncertainty float64 `json:"uncertainty"`
			Settling    bool    `json:"settling"`
		}
		if json.Unmarshal(sc.Bytes(), &rec) != nil || rec.T == "" {
			continue
		}
		t, err := time.Parse(time.RFC3339Nano, rec.T)
		if err != nil {
			continue
		}
		edges = append(edges, replayEdge{
			t:           t,
			uncertainty: time.Duration(rec.Uncertainty * float64(time.Second)),
			settling:    rec.Settling,
		})
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if len(edges) == 0 {
		return nil, fmt.Errorf("%s contains no PPS edge records", path)
	}
	return edges, nil
}
