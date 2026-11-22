package main

import (
	"context"
	"errors"
	"log/slog"
	"syscall/js"
	"testing"

	"github.com/jclark/satpulse/gps/app/gpscfg"
	"github.com/jclark/satpulse/gps/app/gpsio"
	"github.com/jclark/satpulse/gps/scan"
)

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
	for _, tc := range []struct {
		name   string
		reject string
		expect error
	}{
		{"abort", "signal.reason", context.Canceled},
		{"device error", `Object.assign(new Error("Port busy"), {name: "NetworkError"})`, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			started := js.FuncOf(func(js.Value, []js.Value) any {
				cancel()
				return nil
			})
			defer started.Release()
			v := js.Global().Get("Object").New()
			v.Set("started", started)
			v.Set("open", js.Global().Get("Function").New("device", "speed", "signal", `
				const p = new Promise((resolve, reject) => {
					signal.addEventListener("abort", () => reject(`+tc.reject+`), {once: true});
				});
				this.started();
				return p;
			`))
			o := &serialOpener{serial: v, device: "serial:1", speed: 38400}
			_, err := o.Open(ctx, slog.Default())
			if tc.expect != nil {
				if !errors.Is(err, tc.expect) {
					t.Errorf("Open error = %v, expect %v", err, tc.expect)
				}
			} else if err == nil || err.Error() != "Port busy" {
				t.Errorf("Open error = %v, expect the device error", err)
			}
		})
	}
}
