package gpsevent

import (
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/gpsreg"
	"github.com/jclark/satpulse/gps/scan"
	"github.com/jclark/satpulse/time/internal/obs"
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
