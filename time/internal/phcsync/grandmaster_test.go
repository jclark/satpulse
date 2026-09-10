package phcsync_test

import (
	"io"
	"log/slog"
	"net"
	"os"
	"syscall"
	"testing"
	"testing/synctest"
	"time"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/gpsreg"
	"github.com/jclark/satpulse/gps/ptime"
	"github.com/jclark/satpulse/time/clocksim"
	"github.com/jclark/satpulse/time/internal/obs"
	"github.com/jclark/satpulse/time/internal/phcsync"
	"github.com/jclark/satpulse/time/internal/ptpgm"
	"github.com/jclark/satpulse/time/internal/timemsg"
	"github.com/jclark/satpulse/time/lib/pmc"
	"github.com/jclark/satpulse/time/phctime"
)

// Exercise retry through Controller.Tick, with continuing samples but no mode
// or leap-information changes. Some cases cross a time-dependent leap boundary
// so retrying the original snapshot would send the wrong settings.
func TestControllerGrandmasterRetry(t *testing.T) {
	positive := ptime.LeapSecondOnDate(time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC), 37, 38)
	negative := ptime.LeapSecondOnDate(time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC), 37, 36)
	for _, tt := range []struct {
		name     string
		leap     ptime.LeapSecond
		ref      ptime.Time
		initial  pmc.TimeFlags
		wantLeap pmc.TimeFlags
		wantUTC  int16
	}{
		{"Unchanged", positive, positive.OffChangeTime.Add(-24 * time.Hour), 0, 0, 37},
		{"LeapAnnouncement", positive, positive.OffChangeTime.Add(-12*time.Hour - 36*time.Second), 0, pmc.Leap61, 37},
		{"PositiveLeap", positive, positive.OffChangeTime.Add(-36 * time.Second), pmc.Leap61, 0, 38},
		{"NegativeLeap", negative, negative.OffChangeTime.Add(-36 * time.Second), pmc.Leap59, 0, 36},
	} {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				f := newGrandmasterController(t, tt.leap)
				f.track(t, tt.ref)
				// The boundary cases place the leap boundary 36 s after tt.ref,
				// so the first request must precede it and the retry 30 s
				// later must follow it, with a margin for sample alignment.
				if e := f.elapsed; e < 8*time.Second || e > 34*time.Second {
					t.Fatalf("tracking reached after %v, want between 8s and 34s", e)
				}
				first := f.request(t)
				checkGrandmasterSettings(t, first.settings, 37, tt.initial, true)
				first.complete(syscall.ENOENT)
				f.tick()
				f.noRequest(t)

				f.advance(30*time.Second - 250*time.Millisecond)
				f.noRequest(t)
				f.advance(250 * time.Millisecond)
				retry := f.request(t)
				checkGrandmasterSettings(t, retry.settings, tt.wantUTC, tt.wantLeap, true)
				retry.complete(nil)

				f.advance(31 * time.Second)
				f.noRequest(t)
				if f.c.Mode() != phcsync.ModeTracking {
					t.Fatalf("mode = %v, want tracking throughout retry", f.c.Mode())
				}
			})
		})
	}
}

// A target returning to an earlier confirmed value must be sent after the
// outstanding SET, regardless of whether that outstanding operation succeeds.
func TestControllerGrandmasterChangedTarget(t *testing.T) {
	for _, outcome := range []struct {
		name string
		err  error
	}{{"Success", nil}, {"Failure", syscall.ENOENT}} {
		t.Run(outcome.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				leap := ptime.LeapSecond{UTCOffAfter: 37}
				f := newGrandmasterController(t, leap)
				f.track(t, ptime.Time(time.Now().UnixNano()))
				first := f.request(t)
				first.complete(nil)
				f.tick()

				f.c.LeapSecond(ptime.LeapSecond{UTCOffAfter: 38})
				pending := f.request(t)
				checkGrandmasterSettings(t, pending.settings, 38, 0, true)
				f.c.LeapSecond(leap)
				f.noRequest(t)
				pending.complete(outcome.err)
				f.noRequest(t)

				f.advance(250 * time.Millisecond)
				latest := f.request(t)
				if latest.settings != first.settings {
					t.Fatalf("follow-up settings = %+v, want %+v", latest.settings, first.settings)
				}
				latest.complete(nil)
				f.advance(31 * time.Second)
				f.noRequest(t)
			})
		})
	}
}

func TestControllerGrandmasterClose(t *testing.T) {
	t.Run("AfterOlderRequest", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			f := newGrandmasterController(t, ptime.LeapSecond{UTCOffAfter: 37})
			f.track(t, ptime.Time(time.Now().UnixNano()))
			old := f.request(t)
			closed := make(chan struct{})
			go func() {
				f.close()
				close(closed)
			}()
			synctest.Wait()
			select {
			case <-closed:
				t.Fatal("controller closed before the older request completed")
			default:
			}

			time.Sleep(90 * time.Millisecond)
			old.complete(syscall.ENOENT)
			<-closed
			final := f.request(t)
			checkGrandmasterSettings(t, final.settings, 37, 0, false)
			if remaining := final.deadline.Sub(time.Now()); remaining != 100*time.Millisecond {
				t.Fatalf("final operation allowance = %v, want 100ms", remaining)
			}
			// Complete after the old deadline to verify the final operation has
			// its own allowance and is not cancelled by controller shutdown.
			time.Sleep(90 * time.Millisecond)
			final.complete(nil)
			<-f.workerDone
			f.noRequest(t)
		})
	})

	t.Run("NoSyncAlreadyPending", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			f := newGrandmasterController(t, ptime.LeapSecond{UTCOffAfter: 37})
			f.track(t, ptime.Time(time.Now().UnixNano()))
			f.request(t).complete(nil)
			f.tick()

			f.c.Pause()
			pending := f.request(t)
			checkGrandmasterSettings(t, pending.settings, 37, 0, false)
			f.close()
			f.noRequest(t)
			pending.complete(nil)
			<-f.workerDone
			f.noRequest(t)
		})
	})
}

func checkGrandmasterSettings(t *testing.T, got pmc.GrandmasterSettings, utc int16, leap pmc.TimeFlags, inSync bool) {
	t.Helper()
	want := pmc.GrandmasterSettings{
		ClockQuality: pmc.ClockQuality{
			ClockClass:              pmc.ClockClassDegradedA,
			ClockAccuracy:           pmc.ClockAccuracyUnknown,
			OffsetScaledLogVariance: pmc.OffsetScaledLogVarianceUnknown,
		},
		UTCOffset:  utc,
		TimeFlags:  pmc.CurrentUTCOffsetValid | pmc.PTPTimescale | leap,
		TimeSource: pmc.TimeSourceGNSS,
	}
	if inSync {
		want.ClockQuality = testGrandmasterQuality
		want.TimeFlags |= pmc.TimeTraceable | pmc.FrequencyTraceable
	}
	if got != want {
		t.Fatalf("grandmaster settings = %+v, want %+v", got, want)
	}
}

var testGrandmasterQuality = pmc.ClockQuality{
	ClockClass:              pmc.ClockClassSyncPrimaryRef,
	ClockAccuracy:           pmc.ClockAccuracyWithin100ns,
	OffsetScaledLogVariance: 0x1234,
}

type grandmasterController struct {
	c          *phcsync.Controller
	conn       *grandmasterConn
	workerDone chan struct{}
	closed     bool
	vclock     *clocksim.VirtualClock
	clock      *clocksim.TestClock
	buffer     *timemsg.Buffer
	ref        ptime.Time
	elapsed    time.Duration
	start      time.Time
}

// Only this wiring depends on the current Grandmaster/worker architecture.
// Tests drive controller events and inspect protocol settings, not requests or
// retry bookkeeping. The transport uses channels so synctest can control time.
func newGrandmasterController(t *testing.T, leap ptime.LeapSecond) *grandmasterController {
	t.Helper()
	gm, requests := ptpgm.NewGrandmaster(ptpgm.Config{InSyncClockQuality: testGrandmasterQuality})
	lg := slog.New(slog.NewTextHandler(io.Discard, nil))
	vclock := clocksim.NewVirtualClock(clocksim.NewRawClock(clocksim.PerfectOsc(), 0), nil, clocksim.PerfectGPS(), 0, 500000, 0, nil)
	clock := clocksim.NewTestClock(vclock)
	c, err := phcsync.NewController(clock, &obs.DefaultObserver{}, gm, phcsync.DefaultConfig(), leap, 1, lg)
	if err != nil {
		t.Fatal(err)
	}
	buffer := timemsg.NewBuffer(lg, c.RequiredMsgWindow(), leap, gpsprot.GPS)
	c.SetTimeMsgBuffer(buffer)
	conn := &grandmasterConn{requests: make(chan *grandmasterExchange, 8)}
	client := &pmc.Client{T: &pmc.Transport{Conn: conn, RemoteAddr: conn.LocalAddr()}}
	f := &grandmasterController{
		c: c, conn: conn, workerDone: make(chan struct{}),
		vclock: vclock, clock: clock, buffer: buffer, start: time.Now(),
	}
	go func() {
		ptpgm.PTP4LWorker(client, requests, lg)
		close(f.workerDone)
	}()
	t.Cleanup(func() {
		f.close()
		// Unanswered operations time out on the bubble's fake clock.
		<-f.workerDone
	})
	return f
}

func (f *grandmasterController) track(t *testing.T, ref ptime.Time) {
	t.Helper()
	f.ref = ref
	for f.c.Mode() != phcsync.ModeTracking {
		if f.elapsed > time.Minute {
			t.Fatal("controller did not reach tracking")
		}
		f.step()
	}
}

// Feed pulses, time messages and ticks through the public controller API.
// The simulated PHC and worker deadlines advance with synctest's clock.
func (f *grandmasterController) advance(d time.Duration) {
	end := f.elapsed + d
	for f.elapsed < end {
		f.step()
	}
}

func (f *grandmasterController) tick() {
	f.c.Tick(f.start.Add(f.elapsed))
}

func (f *grandmasterController) step() {
	time.Sleep(250 * time.Millisecond)
	f.elapsed += 250 * time.Millisecond
	f.vclock.AdvanceTo(f.elapsed.Seconds())
	sys := f.start.Add(f.elapsed)
	for f.vclock.TimestampAvailable() {
		reading, _ := f.clock.ReadTimestamp()
		f.c.PulseEdge(phcsync.PulseEdge{
			Timestamp: reading.Timestamp,
			TRead:     phctime.Sample{PHC: f.clock.Now(), Sys: sys},
		})
	}
	// Deliver the navigation time message 250ms after each pulse.
	if f.elapsed > time.Second && f.elapsed%time.Second == 250*time.Millisecond {
		f.buffer.Time(&gpsprot.TimeMsg{
			TAITime: f.ref.Add(f.elapsed.Truncate(time.Second)),
			GNSS:    gpsprot.GPS, Ref: gpsprot.PostPulse,
			Tag: gpsreg.TagUBX, NativeMsgID: "NAV-PVT",
		}, sys)
		f.c.TimeMessage()
	}
	f.tick()
	synctest.Wait()
}

func (f *grandmasterController) close() {
	if !f.closed {
		f.closed = true
		f.c.Close()
	}
}

func (f *grandmasterController) request(t *testing.T) *grandmasterExchange {
	t.Helper()
	synctest.Wait()
	select {
	case req := <-f.conn.requests:
		return req
	default:
		t.Fatal("controller did not send a grandmaster update")
		return nil
	}
}

func (f *grandmasterController) noRequest(t *testing.T) {
	t.Helper()
	synctest.Wait()
	select {
	case req := <-f.conn.requests:
		t.Fatalf("unexpected grandmaster update: %+v", req.settings)
	default:
	}
}

type grandmasterExchange struct {
	settings pmc.GrandmasterSettings
	deadline time.Time
	result   chan error
}

func (e *grandmasterExchange) complete(err error) {
	e.result <- err
	synctest.Wait()
}

// grandmasterConn implements the PMC transport without sockets. Each send
// blocks until the test completes it or the worker's deadline expires. A
// successful send gets an echo response, just as ptp4l responds to SET.
type grandmasterConn struct {
	requests chan *grandmasterExchange
	deadline time.Time
	response []byte
}

func (c *grandmasterConn) WriteTo(data []byte, addr net.Addr) (int, error) {
	msg, err := pmc.UnmarshalMgmtMsg(data)
	if err != nil {
		return 0, err
	}
	settings := msg.(*pmc.GrandmasterSettingsMsg)
	req := &grandmasterExchange{settings: settings.V.Data, deadline: c.deadline, result: make(chan error, 1)}
	c.requests <- req
	timer := time.NewTimer(time.Until(c.deadline))
	defer timer.Stop()
	select {
	case err := <-req.result:
		if err != nil {
			return 0, err
		}
	case <-timer.C:
		return 0, os.ErrDeadlineExceeded
	}
	settings.ActionField = pmc.ActionResponse
	settings.TargetPortIdentity = settings.SourcePortIdentity
	c.response, err = settings.MarshalBinary()
	return len(data), err
}

func (c *grandmasterConn) ReadFrom(data []byte) (int, net.Addr, error) {
	return copy(data, c.response), c.LocalAddr(), nil
}

func (*grandmasterConn) Close() error { return nil }
func (*grandmasterConn) LocalAddr() net.Addr {
	return &net.UnixAddr{Name: "fake-ptp4l", Net: "unixgram"}
}
func (c *grandmasterConn) SetDeadline(t time.Time) error {
	c.deadline = t
	return nil
}
func (c *grandmasterConn) SetReadDeadline(t time.Time) error  { return c.SetDeadline(t) }
func (c *grandmasterConn) SetWriteDeadline(t time.Time) error { return c.SetDeadline(t) }
