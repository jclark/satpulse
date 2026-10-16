package ubxsim

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"math"
	"net"
	"testing"
	"testing/synctest"
	"time"

	"github.com/jclark/satpulse/gps/lib/ubxbin"
	ucv "github.com/jclark/satpulse/gps/lib/ubxcfgval"
)

func TestClockFit(t *testing.T) {
	const utc0 int64 = 1792108800000000000
	const raw0 int64 = 82000000000000
	for _, ppm := range []int64{-150, 0, 200} {
		var fit clockFit
		for i := range 250 {
			d := int64(i) * 50_000_000
			noise := int64(i%5-2) * 10
			if fit.add(utc0+d, raw0+d+d*ppm/1_000_000+noise) {
				t.Fatalf("unexpected discontinuity at sample %d", i)
			}
			if !fit.ready() {
				continue
			}
			d += 900_000_000
			want := raw0 + d + d*ppm/1_000_000
			if delta := fit.rawAt(utc0+d) - want; math.Abs(float64(delta)) > 50 {
				t.Fatalf("ppm=%d sample=%d: prediction error %dns", ppm, i, delta)
			}
		}
		if !fit.add(utc0+13_000_000_000, raw0+12_500_000_000) {
			t.Fatal("UTC step not detected")
		}
	}
}

func TestTimingMessages(t *testing.T) {
	// One second before GPS week 2441: TIM-TP must roll over while all
	// navigation messages and TIM-TOS still describe the preceding second.
	utc := time.Date(2026, 10, 17, 23, 59, 41, 0, time.UTC)
	msgs := timingMessages(utc, 18)
	for i, m := range msgs {
		b, err := ubxbin.Serialize(m)
		if err != nil {
			t.Fatal(err)
		}
		msgs[i], err = ubxbin.ParseMsg(string(b))
		if err != nil {
			t.Fatal(err)
		}
	}
	gps := msgs[0].(*ubxbin.NavTimeGPS)
	if gps.Week != 2440 || gps.ITOW != 604799000 || gps.LeapS != 18 || gps.Valid != 7 || gps.FTOW != 0 {
		t.Fatalf("TIMEGPS: %+v", gps)
	}
	u := msgs[1].(*ubxbin.NavTimeUTC)
	if u.Year != 2026 || u.Month != 10 || u.Day != 17 || u.Hour != 23 || u.Min != 59 || u.Sec != 41 || u.Nano != 0 || u.Valid != 0x37 || u.ITOW != gps.ITOW {
		t.Fatalf("TIMEUTC: %+v", u)
	}
	pvt := msgs[2].(*ubxbin.NavPVT)
	if pvt.Sec != u.Sec || pvt.ITOW != gps.ITOW || pvt.Valid != 7 || pvt.Flags2 != 0xe0 || pvt.FixType != ubxbin.NavPVTTimeOnlyFix {
		t.Fatalf("PVT: %+v", pvt)
	}
	tos := msgs[3].(*ubxbin.TimTos)
	if tos.Second != u.Sec || tos.Week != uint32(gps.Week) || tos.TOW != gps.ITOW/1000 || tos.UTCOffset != 0 || tos.GNSSOffset != 0 || tos.Flags&0xc0 != 0xc0 {
		t.Fatalf("TOS: %+v", tos)
	}
	tp := msgs[4].(*ubxbin.TimTP)
	if tp.Week != 2441 || tp.TOWMS != 0 || tp.TOWSubMS != 0 || tp.QErr != 0 || tp.Flags != ubxbin.TimTPUTCAvailable || tp.RefInfo != ubxbin.TimTPTimeRefGPS {
		t.Fatalf("TP: %+v", tp)
	}
}

func TestTimingEdgesAndOutput(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		db := newCfgDB(ucv.Map{
			ucv.KUsboutprotUbx.Key():               1,
			ucv.KUbxNavTimegps.KeyU(ucv.USB).Key(): 1,
			ucv.KUbxTimTp.KeyU(ucv.USB).Key():      2,
		})
		w := newWriter(io.Discard, db, ucv.USB)
		n := &navEngine{db: db, port: ucv.USB, w: w, lg: slog.New(slog.DiscardHandler), reboot: make(chan struct{}, 1)}
		start := time.Now()
		const utc0 int64 = 1792108800000000000
		const raw0 int64 = 8000000000
		var edges []int64
		timing := &Timing{
			LeapSeconds: 18,
			ReadTime: func() (int64, int64, error) {
				d := time.Since(start).Nanoseconds()
				return utc0 + d, raw0 + d + d/10000, nil
			},
			ScheduleEdge: func(raw int64) error {
				d := time.Since(start).Nanoseconds()
				now := raw0 + d + d/10000
				if raw <= now || (len(edges) != 0 && edges[len(edges)-1] > now) {
					t.Fatalf("replacement would lose an edge: now=%d previous=%v next=%d", now, edges, raw)
				}
				edges = append(edges, raw)
				return nil
			},
		}
		done := make(chan error, 1)
		go func() { done <- n.runTiming(ctx, timing) }()
		time.Sleep(2*time.Second + 99*time.Millisecond)
		synctest.Wait()
		if len(edges) != 1 || edges[0] != raw0+2_000_200_000 || len(w.ch) != 0 {
			t.Fatalf("before delay: edges=%v packets=%d", edges, len(w.ch))
		}
		time.Sleep(time.Millisecond)
		synctest.Wait()
		if len(edges) != 2 || edges[1] != raw0+3_000_300_000 || len(w.ch) != 2 {
			t.Fatalf("after delay: edges=%v packets=%d", edges, len(w.ch))
		}
		for _, id := range []ubxbin.MsgID{ubxbin.NavTimeGPSID, ubxbin.TimTPID} {
			if p := <-w.ch; ubxbin.PacketMsgId(string(p.data)) != id {
				t.Fatalf("unexpected output message %v", ubxbin.PacketMsgId(string(p.data)))
			}
		}
		time.Sleep(time.Second)
		synctest.Wait()
		if len(w.ch) != 1 {
			t.Fatalf("rate 2 TIM-TP emitted on odd epoch: %d packets", len(w.ch))
		}
		cancel()
		if err := <-done; !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	})
}

func TestTimingFailureStopsSim(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		a, b := net.Pipe()
		defer a.Close()
		defer b.Close()
		want := errors.New("reference failed")
		s := New(&Personality{}, Options{Timing: &Timing{
			ReadTime:     func() (int64, int64, error) { return 0, 0, want },
			ScheduleEdge: func(int64) error { return nil },
		}})
		if err := s.Run(context.Background(), a); !errors.Is(err, want) {
			t.Fatalf("Run: %v", err)
		}
	})
}
