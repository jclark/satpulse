package main

import (
	"context"
	"errors"
	"log/slog"
	"syscall/js"
	"testing"
	"time"

	"github.com/jclark/satpulse/gps/app/gpscfg"
	"github.com/jclark/satpulse/gps/app/gpsio"
	"github.com/jclark/satpulse/gps/scan"
)

func TestAwaitRejections(t *testing.T) {
	obj := js.Global().Get("Object").New()
	for _, tc := range []struct {
		name string
		v    js.Value
		want string
	}{
		{"string", js.ValueOf("failed"), "failed"},
		{"number", js.ValueOf(17), "17"},
		{"boolean", js.ValueOf(false), "false"},
		{"null", js.Null(), "null"},
		{"undefined", js.Undefined(), "undefined"},
		{"object", obj, "[object Object]"},
		{"error", js.Global().Get("Error").New("failed"), "failed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := await(js.Global().Get("Promise").Call("reject", tc.v))
			if err == nil || err.Error() != tc.want {
				t.Fatalf("rejection = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestSerialReadErrors(t *testing.T) {
	for _, name := range []string{"FramingError", "ParityError", "BreakError", "BufferOverrunError", "NetworkError"} {
		t.Run(name, func(t *testing.T) {
			e := js.Global().Get("Error").New("serial fault")
			e.Set("name", name)
			read := js.FuncOf(func(js.Value, []js.Value) any {
				return js.Global().Get("Promise").Call("reject", e)
			})
			defer read.Release()
			v := js.Global().Get("Object").New()
			v.Set("read", read)
			c := &serialConn{value: v, op: &serialOpener{device: "serial:1"}}
			s := scan.New(c, 16, nil)
			pkt, err := s.Scan()
			if name == "NetworkError" {
				if err == nil || pkt.ReadError != nil {
					t.Fatalf("terminal fault: packet error %v, scan error %v", pkt.ReadError, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("temporary fault stopped scanner: %v", err)
			}
			var se *gpsio.SerialError
			if !errors.As(pkt.ReadError, &se) || !se.Temporary() || se.Counts != nil {
				t.Fatalf("packet error = %#v, want serial error without counts", pkt.ReadError)
			}
			framing, ok := pkt.ReadError.(gpscfg.SerialError)
			if !ok || framing.SerialFraming() != (name == "FramingError") {
				t.Fatalf("framing contract: error = %v", pkt.ReadError)
			}
		})
	}
}

func TestOpenContextCancellation(t *testing.T) {
	v := js.Global().Get("Object").New()
	v.Set("open", js.Global().Get("Function").New("device", "speed", "signal", `
		return new Promise((resolve, reject) => {
			signal.addEventListener("abort", () => reject(signal.reason), {once: true});
		});
	`))
	o := &serialOpener{serial: v, device: "serial:1", speed: 38400}
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()
	_, err := o.Open(ctx, slog.Default())
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Open error = %v, want context deadline", err)
	}
}
