package pps

import (
	"context"
	"fmt"
	"reflect"
	"testing"
	"time"
	"unsafe"

	"github.com/jclark/satpulse/time/lib/median"
)

// stepWall constructs a clock step without changing the host clock. time.Time
// has no public constructor for independent wall and monotonic readings, so
// restore its monotonic field after Add advances both. Verify the resulting
// readings so a change to time.Time's representation fails the test clearly.
func stepWall(t *testing.T, v time.Time, step time.Duration) time.Time {
	t.Helper()
	stepped := v.Add(step)
	ext := reflect.ValueOf(&stepped).Elem().FieldByName("ext")
	if !ext.IsValid() || ext.Kind() != reflect.Int64 || stepped == stepped.Round(0) {
		t.Fatal("time.Time has no expected monotonic field")
	}
	*(*int64)(unsafe.Pointer(ext.UnsafeAddr())) -= int64(step)
	if stepped.Sub(v) != 0 || stepped.Round(0).Sub(v.Round(0)) != step {
		t.Fatal("failed to construct independent wall and monotonic readings")
	}
	return stepped
}

func TestPollWindowClockStep(t *testing.T) {
	for _, acquired := range []bool{false, true} {
		for _, step := range []time.Duration{-100 * time.Millisecond, 100 * time.Millisecond} {
			for split := 1; split < 4; split++ {
				t.Run(fmt.Sprintf("tracking_%v/step_%s/split_%d", acquired, step, split), func(t *testing.T) {
					base := time.Now()
					var reads []time.Time
					for cycle := range 2 {
						for i, at := range []time.Duration{0, 4803, 6370, 10810} {
							v := base.Add(time.Duration(cycle)*period + at)
							if cycle > 0 || i >= split {
								v = stepWall(t, v, step)
							}
							reads = append(reads, v)
						}
					}
					calls := 0
					candidates := make(chan CandidateEdge, 1)
					p := poller{
						ctx: context.Background(), lg: testLog, ceCh: candidates, nextEdge: base,
						widths: median.New[time.Duration](widthHistory),
						r:      pulseReaderFunc(func() (bool, error) { calls++; return calls%2 == 0, nil }),
						params: PollParams{
							MinSpacing: minSpacing,
							Now:        func() time.Time { v := reads[0]; reads = reads[1:]; return v },
							Wait:       func(context.Context, time.Time, bool) (bool, error) { return false, nil },
						},
					}
					for cycle := range 2 {
						prediction := base.Add(time.Duration(cycle) * period)
						p.nextEdge = prediction
						o, predictionError, width, err := p.pollWindow(time.Millisecond, minSpacing, acquired)
						if err != nil || o != caught || predictionError != 5495 {
							t.Fatalf("cycle %d: pollWindow = %v, %v, %v; want catch, 5495ns, nil", cycle, o, predictionError, err)
						}
						ce := <-candidates
						wantReject := RejectClockStep
						wantStamp := prediction.Round(0).Add(5495)
						if cycle > 0 {
							wantReject = ""
							if !acquired {
								wantReject = RejectAcquiring
							}
							wantStamp = wantStamp.Add(step)
						}
						if ce.Reject != wantReject || !ce.Timestamp.Equal(wantStamp) {
							t.Errorf("cycle %d: stamp %v reject %q; want %v, %q", cycle, ce.Timestamp, ce.Reject, wantStamp, wantReject)
						}
						if ce.Uncertainty != [2]time.Duration{5495, 5315} || ce.PollWidths != [2]time.Duration{4803, 4440} || ce.ReadDelay != 5315 || width != 10810 {
							t.Errorf("clock step changed monotonic intervals: %+v; width %v", ce, width)
						}
						wantNext := prediction
						if !acquired {
							wantNext = prediction.Add(period + 5495)
						}
						if p.nextEdge.Sub(wantNext) != 0 {
							t.Errorf("nextEdge = %v, want monotonic prediction %v", p.nextEdge, wantNext)
						}
					}
				})
			}
		}
	}
}
