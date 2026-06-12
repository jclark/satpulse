package gpsevent

import (
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/gpsreg"
	"github.com/jclark/satpulse/gps/ptime"
	"github.com/jclark/satpulse/gps/scan"
	"github.com/jclark/satpulse/time/internal/obs"
	"github.com/jclark/satpulse/time/internal/refclock"
	"github.com/jclark/satpulse/time/lib/ntime"
	"github.com/jclark/satpulse/time/lib/ntpshm"
)

type nativeMsgObserver struct {
	obs.DefaultObserver
	handled bool
	count   int
	tag     gpsprot.Tag
	msgID   string
	msg     any
	tRead   time.Time
}

func (o *nativeMsgObserver) NativeMsg(tag gpsprot.Tag, msgID string, msg any, tRead time.Time) bool {
	o.count++
	o.tag = tag
	o.msgID = msgID
	o.msg = msg
	o.tRead = tRead
	return o.handled
}

func TestDispatcherNativeMsgForwardsToObserver(t *testing.T) {
	obs := &nativeMsgObserver{handled: true}
	d := &Dispatcher{
		obs: obs,
		lg:  slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	tRead := time.Unix(1, 2)
	msg := struct{ Value int }{Value: 42}

	if err := d.NativeMsg("TEST", "MSG", msg, tRead); err != nil {
		t.Fatalf("NativeMsg returned error: %v", err)
	}

	if obs.count != 1 {
		t.Fatalf("NativeMsg observer count = %d, want 1", obs.count)
	}
	if obs.tag != "TEST" || obs.msgID != "MSG" || obs.msg != msg || !obs.tRead.Equal(tRead) {
		t.Fatalf("NativeMsg observer got (%q, %q, %#v, %v), want (%q, %q, %#v, %v)",
			obs.tag, obs.msgID, obs.msg, obs.tRead, gpsprot.Tag("TEST"), "MSG", msg, tRead)
	}
}

type corReportObserver struct {
	obs.DefaultObserver
	msgs  []*gpsprot.CorReportMsg
	tRead []time.Time
}

func (o *corReportObserver) CorReport(msg *gpsprot.CorReportMsg, tRead time.Time) {
	o.msgs = append(o.msgs, msg)
	o.tRead = append(o.tRead, tRead)
}

const testRTCM1005 = "\xD3\x00\x13\x3E\xD7\xD3\x02\x02\x98\x0E\xDE\xEF\x34\xB4\xBD\x62\xAC\x09\x41\x98\x6F\x33\x36\x0B\x98"

func TestDispatcherRunEmitsPulledRTCMCorReport(t *testing.T) {
	observer := &corReportObserver{}
	d := &Dispatcher{
		obs: observer,
		lg:  slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	pktCh := make(chan scan.Packet)
	close(pktCh)
	pullPktCh := make(chan scan.Packet, 1)
	tRead := time.Unix(1, 2)
	pullPktCh <- scan.Packet{
		Format:        gpsreg.RTCMPacketFormat,
		Data:          testRTCM1005,
		TRead:         tRead,
		ChecksumValid: true,
	}
	close(pullPktCh)

	d.Run(nil, pktCh, pullPktCh)

	if len(observer.msgs) != 1 {
		t.Fatalf("observer got %d CorReport messages, want 1", len(observer.msgs))
	}
	msg := observer.msgs[0]
	if msg.Source != gpsprot.CorReportSourcePull || msg.Tag != gpsreg.TagRTCM || msg.MsgID != "1005" {
		t.Fatalf("CorReport = source %v tag %q msgID %q, want pull RTCM 1005",
			msg.Source, msg.Tag, msg.MsgID)
	}
	if !msg.NBytes.IsSet() || msg.NBytes.Get() != len(testRTCM1005) {
		t.Errorf("NBytes = (%d, %v), want set %d", msg.NBytes.Get(), msg.NBytes.IsSet(), len(testRTCM1005))
	}
	if !msg.ChecksumOK.IsSet() || !msg.ChecksumOK.Get() {
		t.Errorf("ChecksumOK = (%v, %v), want set true", msg.ChecksumOK.Get(), msg.ChecksumOK.IsSet())
	}
	if !observer.tRead[0].Equal(tRead) {
		t.Errorf("tRead = %v, want %v", observer.tRead[0], tRead)
	}
}

type shmWrite struct {
	clock     time.Time
	receive   time.Time
	leap      ptime.LeapSecondKind
	precision int8
}

type fakeSHM struct {
	writes    []shmWrite
	precision int8
}

func (s *fakeSHM) Write(clockTime, receiveTime time.Time, leap ptime.LeapSecondKind) {
	s.writes = append(s.writes, shmWrite{
		clock:     clockTime,
		receive:   receiveTime,
		leap:      leap,
		precision: s.precision,
	})
}

func (s *fakeSHM) Close() error { return nil }

type ntpSampleObserver struct {
	obs.DefaultObserver
	count  int
	sys    time.Time
	offset float64
	leap   ptime.LeapSecondKind
	phc    ptime.Time
}

func (o *ntpSampleObserver) NTPSample(sys time.Time, offset float64, leap ptime.LeapSecondKind, phc ptime.Time) {
	o.count++
	o.sys = sys
	o.offset = offset
	o.leap = leap
	o.phc = phc
}

func TestDispatcherMsgUTCTimeWritesSHM(t *testing.T) {
	shm := &fakeSHM{precision: -1}
	observer := &ntpSampleObserver{}
	d := &Dispatcher{
		shm: shm,
		obs: observer,
		lg:  slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	utc := time.Unix(100, 123)
	tRead := time.Unix(99, 456)
	d.MsgUTCTime(utc, tRead, ptime.LeapSecondPositive)
	if len(shm.writes) != 1 {
		t.Fatalf("SHM writes = %d, want 1", len(shm.writes))
	}
	w := shm.writes[0]
	if !w.clock.Equal(utc) || !w.receive.Equal(tRead) || w.leap != ptime.LeapSecondPositive || w.precision != -1 {
		t.Fatalf("SHM write = %+v, want clock %v receive %v leap positive precision -1", w, utc, tRead)
	}
	if observer.count != 1 || !observer.sys.Equal(tRead) || observer.leap != ptime.LeapSecondPositive || observer.phc != 0 {
		t.Fatalf("observer sample = count %d sys %v leap %v phc %v", observer.count, observer.sys, observer.leap, observer.phc)
	}
	if want := utc.Sub(tRead).Seconds(); observer.offset != want {
		t.Fatalf("observer offset = %v, want %v", observer.offset, want)
	}
}

func TestDispatcherMsgUTCTimeWritesBothSinks(t *testing.T) {
	shm := &fakeSHM{precision: -7}
	rc, ch := refclock.NewProxyRefClock()
	defer rc.Close()
	d := &Dispatcher{
		rc:  rc,
		shm: shm,
		obs: &obs.DefaultObserver{},
		lg:  slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	utc := time.Unix(200, 0)
	tRead := time.Unix(199, 0)
	d.MsgUTCTime(utc, tRead, ptime.LeapSecondNegative)
	if len(shm.writes) != 1 {
		t.Fatalf("SHM writes = %d, want 1", len(shm.writes))
	}
	select {
	case s := <-ch:
		if s.Sys != ntime.Sys(tRead) || s.Offset != utc.Sub(tRead).Seconds() || s.Leap != ptime.LeapSecondNegative {
			t.Fatalf("refclock sample = %+v", s)
		}
	default:
		t.Fatalf("refclock sample was not sent")
	}
}

func TestDispatcherSHMPrecisionLifecycle(t *testing.T) {
	base := &ntpshm.Writer{}
	shm := NewSHMWriter(base, nil)
	setter, ok := shm.(samplePrecisionSetter)
	if !ok {
		t.Fatalf("SHM writer is %T, want samplePrecisionSetter", shm)
	}
	cw := shm.(*calibratingSHMWriter)
	setter.setSamplePrecision(20 * time.Nanosecond)
	if cw.win == nil || cw.win.Len() != 1 {
		t.Fatalf("precision window length = %v, want 1", cw.win)
	}
	for i := 1; i < precisionCalibrationSamples-1; i++ {
		setter.setSamplePrecision(5 * time.Nanosecond)
		if cw.win == nil {
			t.Fatalf("precision window released before calibration on sample %d", i+1)
		}
	}
	setter.setSamplePrecision(2 * time.Microsecond)
	if cw.win != nil {
		t.Fatalf("precision window was not released")
	}
	setter.setSamplePrecision(time.Second)
}

func TestDispatcherSHMPrecisionOverride(t *testing.T) {
	precision := int8(12)
	base := &ntpshm.Writer{}
	shm := NewSHMWriter(base, &precision)
	if _, ok := shm.(samplePrecisionSetter); ok {
		t.Fatalf("explicit precision writer unexpectedly has a calibration setter")
	}
	if shm != base {
		t.Fatalf("explicit precision writer = %T, want base writer", shm)
	}
}
