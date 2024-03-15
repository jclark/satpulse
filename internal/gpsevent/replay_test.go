package gpsevent

import (
	"bufio"
	"io"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/jclark/satpulse/internal/combine"
	"github.com/jclark/satpulse/internal/phc"
	"github.com/jclark/satpulse/internal/ptime"
)

func TestReplayFast(t *testing.T) {
	testReplay(t, "testdata/fast.jsonl", phc.DriverBothEdges, 34)
}

func testReplay(t *testing.T, fn string, phcFlags phc.DriverFlags, expectedSampleCount int) {
	sampler, err := replayFile(fn, phcFlags)
	if err != nil {
		t.Fatalf("error replaying %s: %v", fn, err)
	}
	if sampler.sampleCount != expectedSampleCount {
		t.Errorf("too few samples emitted: got %d, want %d", sampler.sampleCount, expectedSampleCount)
	}
}

type replaySampler struct {
	sampleCount int
}

func replayFile(fn string, phcFlags phc.DriverFlags) (*replaySampler, error) {
	pt := combine.PulseType{EdgesPerPulse: phcFlags.Edges(), PulseWidth: time.Second / 10}
	ccfg := combine.Config{}
	ccfg.SetDefault(pt)
	if phcFlags&phc.DriverPoll4Hz != 0 {
		ccfg.PulsePollInterval = time.Second / 4
	}
	sampler := replaySampler{}
	lg := slog.New(slog.NewTextHandler(io.Discard, nil))
	cb, err := combine.NewCombiner(pt, &sampler, lg, ccfg)
	if err != nil {
		return nil, err
	}
	replayer := LogReplayer{tStart: time.Now(), cb: cb}
	f, err := os.Open(fn)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		err := replayer.replayEvent([]byte(line))
		if err != nil {
			return nil, err
		}
	}
	return &sampler, nil
}

func (s *replaySampler) Sample(ref ptime.Time, local ptime.ClockTime, delayed bool) {
	s.sampleCount++
}
