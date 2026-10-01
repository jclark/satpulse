// The wasm shell binds the session to a single browser page. JavaScript owns
// Web Serial stream locks; the Go adapter keeps the session's blocking I/O.
package main

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"syscall/js"

	"github.com/jclark/satpulse/gps/app/session"
	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/gpsreg"
	"github.com/jclark/satpulse/gps/lib/geopos"
	"github.com/jclark/satpulse/gps/msgfile"
)

type shell struct {
	sess    *session.Session
	serial  js.Value
	vendors []gpsreg.Vendor
	mu      sync.Mutex
	device  string
	speed   int
}

type eventSink struct {
	mu        sync.Mutex
	callbacks map[session.EventName]js.Value
}

func main() {
	sink := &eventSink{callbacks: make(map[session.EventName]js.Value)}
	lg := slog.New(session.NewLogHandler(sink, slog.NewTextHandler(io.Discard, nil)))
	s := &shell{sess: session.New(lg, sink, session.Options{})}
	api := js.Global().Get("Object").New()
	api.Set("init", js.FuncOf(func(_ js.Value, args []js.Value) any {
		v, err := gpsreg.ParseVendor(args[1].String())
		if err != nil {
			return err.Error()
		}
		s.vendors = nil
		if v != 0 {
			s.vendors = []gpsreg.Vendor{v}
		}
		s.serial = args[0]
		return nil
	}))
	api.Set("on", js.FuncOf(func(_ js.Value, args []js.Value) any {
		sink.mu.Lock()
		defer sink.mu.Unlock()
		name := session.EventName(args[0].String())
		if args[1].IsNull() {
			delete(sink.callbacks, name)
		} else {
			sink.callbacks[name] = args[1]
		}
		return nil
	}))
	api.Set("call", js.FuncOf(func(_ js.Value, args []js.Value) any {
		name, data := args[0].String(), args[1].String()
		return promise(func() (any, error) { return s.call(name, data) })
	}))
	js.Global().Set("satpulseWorkbench", api)
	js.Global().Call("satpulseWorkbenchReady")
	select {}
}

func (s *shell) call(name, data string) (any, error) {
	switch name {
	case "connection":
		s.mu.Lock()
		defer s.mu.Unlock()
		if speed := s.sess.Speed(); speed != 0 {
			s.speed = speed
		}
		return session.ConnectionInfo{State: s.sess.State(), Device: s.device, Speed: s.speed}, nil
	case "receiver":
		return s.sess.Receiver(), nil
	case "corrections":
		return s.sess.CorrectionsState(), nil
	case "connect":
		var req struct {
			Device string `json:"device"`
			Speed  int    `json:"speed"`
		}
		if err := json.Unmarshal([]byte(data), &req); err != nil {
			return nil, err
		}
		if req.Device == "" || req.Speed <= 0 {
			return nil, fmt.Errorf("select a serial port and a positive speed")
		}
		s.mu.Lock()
		s.device, s.speed = req.Device, req.Speed
		s.mu.Unlock()
		o := &serialOpener{serial: s.serial, device: req.Device, speed: req.Speed}
		return nil, s.sess.Connect(o, s.vendors)
	case "disconnect":
		s.sess.Disconnect()
		return nil, nil
	case "config/read":
		return s.sess.ReadConfig(context.Background())
	case "config/apply":
		t := gpsprot.NewConfigTarget()
		if err := json.Unmarshal([]byte(data), t); err != nil {
			return nil, err
		}
		return nil, s.sess.ApplyConfig(context.Background(), t)
	case "signals":
		var req struct {
			GNSS gpsprot.GNSSSet `json:"gnss"`
		}
		if err := json.Unmarshal([]byte(data), &req); err != nil {
			return nil, err
		}
		return s.sess.SignalCatalog(req.GNSS), nil
	case "decode-packet":
		var req struct {
			Data string `json:"data"`
			Hex  bool   `json:"hex"`
			Out  bool   `json:"out"`
		}
		if err := json.Unmarshal([]byte(data), &req); err != nil {
			return nil, err
		}
		b := []byte(req.Data)
		if req.Hex {
			var err error
			b, err = hex.DecodeString(req.Data)
			if err != nil {
				return nil, err
			}
		}
		return s.sess.DecodePacket(gpsreg.CreatePacketFormats(nil), b, req.Out)
	case "geo/ecef-to-llh", "geo/llh-to-ecef", "geo/check-on-earth", "geo/vel-ned-to-ecef", "geo/vel-ecef-to-ned":
		var v [3]float64
		if err := json.Unmarshal([]byte(data), &v); err != nil {
			return nil, err
		}
		return s.geo(name, v)
	case "msgfile/catalog":
		names := msgfile.ListNames([]msgfile.Dir{msgfile.Builtin()})
		return struct {
			Names     []msgfile.Entry `json:"names"`
			Preselect string          `json:"preselect,omitempty"`
		}{names, s.sess.MsgFilePreselect(names, s.vendors)}, nil
	case "msgfile/select":
		var req msgfile.Name
		if err := json.Unmarshal([]byte(data), &req); err != nil {
			return nil, err
		}
		dir, name, err := msgfile.FindName(req, []msgfile.Dir{msgfile.Builtin()})
		if err != nil {
			return nil, err
		}
		mf, err := dir.Load(name)
		if err != nil {
			return nil, err
		}
		return struct {
			Path string               `json:"path"`
			Tags []session.MsgFileTag `json:"tags"`
		}{dir.DisplayPath(name), s.sess.SetMsgFile(mf)}, nil
	case "msgfile/send":
		var req struct {
			Tag  string `json:"tag"`
			Port string `json:"port"`
			Save bool   `json:"save"`
		}
		if err := json.Unmarshal([]byte(data), &req); err != nil {
			return nil, err
		}
		return nil, s.sess.SendMsgFile(req.Tag, req.Port, req.Save)
	case "msgfile/cancel":
		return nil, s.sess.CancelMsgSend()
	}
	return nil, fmt.Errorf("unknown operation %q", name)
}

func (s *shell) geo(name string, v [3]float64) (any, error) {
	switch name {
	case "geo/ecef-to-llh":
		llh, err := geopos.WGS84.ECEFtoLLH(geopos.ECEF(v))
		return struct {
			Lat    float64 `json:"lat"`
			Lon    float64 `json:"lon"`
			Height float64 `json:"height"`
		}{llh.Lat, llh.Lon, llh.Height}, err
	case "geo/llh-to-ecef":
		return [3]float64(geopos.WGS84.LLHtoECEF(geopos.LLH{Lat: v[0], Lon: v[1], Height: v[2]})), nil
	case "geo/check-on-earth":
		return geopos.ECEF(v).CheckOnEarth() == nil, nil
	case "geo/vel-ned-to-ecef":
		return s.sess.VelNEDtoECEF(v[0], v[1], v[2]), nil
	case "geo/vel-ecef-to-ned":
		return s.sess.VelECEFtoNED(v[0], v[1], v[2]), nil
	}
	panic("unknown geodesy operation")
}

// Emit delivers an event to the JavaScript dispatcher, which queues UI delivery.
func (s *eventSink) Emit(ev session.Event) {
	s.mu.Lock()
	f, ok := s.callbacks[ev.EventName()]
	s.mu.Unlock()
	if !ok {
		return
	}
	b, err := json.Marshal(ev)
	if err == nil {
		f.Invoke(string(b))
	}
}

// Wants reports whether the frontend has subscribed to name.
func (s *eventSink) Wants(name session.EventName) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.callbacks[name]
	return ok
}

func promise(f func() (any, error)) js.Value {
	exec := js.FuncOf(func(_ js.Value, args []js.Value) any {
		resolve, reject := args[0], args[1]
		go func() {
			v, err := f()
			if err == nil {
				var b []byte
				b, err = json.Marshal(v)
				if err == nil {
					resolve.Invoke(string(b))
					return
				}
			}
			reject.Invoke(js.Global().Get("Error").New(err.Error()))
		}()
		return nil
	})
	defer exec.Release()
	return js.Global().Get("Promise").New(exec)
}

type promiseResult struct {
	v   js.Value
	err error
}

type jsError struct {
	name    string
	message string
}

// Error returns the JavaScript rejection's display message.
func (e *jsError) Error() string { return e.message }

func rejection(v js.Value) error {
	e := &jsError{}
	if !v.IsNull() && (v.Type() == js.TypeObject || v.Type() == js.TypeFunction) {
		if name := v.Get("name"); name.Type() == js.TypeString {
			e.name = name.String()
		}
		if message := v.Get("message"); message.Type() == js.TypeString {
			e.message = message.String()
		}
	}
	if e.message == "" {
		e.message = js.Global().Get("String").Invoke(v).String()
	}
	return e
}

func await(p js.Value) (js.Value, error) {
	ch := make(chan promiseResult, 1)
	ok := js.FuncOf(func(_ js.Value, args []js.Value) any {
		v := js.Undefined()
		if len(args) > 0 {
			v = args[0]
		}
		ch <- promiseResult{v: v}
		return nil
	})
	bad := js.FuncOf(func(_ js.Value, args []js.Value) any {
		v := js.Undefined()
		if len(args) > 0 {
			v = args[0]
		}
		ch <- promiseResult{err: rejection(v)}
		return nil
	})
	defer ok.Release()
	defer bad.Release()
	p.Call("then", ok, bad)
	r := <-ch
	return r.v, r.err
}
