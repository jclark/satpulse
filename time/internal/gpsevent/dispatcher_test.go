package gpsevent

import (
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/jclark/satpulse/gps/gpsprot"
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
