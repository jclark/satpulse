//go:build tinygo

package bcast

import (
	"reflect"
	"runtime"
	"unsafe"
)

// selectCases below stands in for reflect.Select, which TinyGo does not
// implement. It is also meant as a prototype of an implementation to push
// upstream, so it follows reflect's semantics and panic wording where it
// can, and the TODOs on selectCases record what an implementation living
// inside reflect would still have to do. Most of the machinery here exists
// only because this code is outside the reflect package and reaches the
// runtime through linkname: upstream would have the channel's element type
// and runtime.alloc to hand, and would need neither the mirrored structs
// below nor the version check.

// These layouts and the linkname signatures mirror TinyGo 0.42.0's
// src/runtime/chan.go. Before supporting another version, check them against
// that runtime and run the select and broadcast tests on TinyGo.
// https://github.com/tinygo-org/tinygo/blob/v0.42.0/src/runtime/chan.go
type tinygoSelectState struct {
	ch    unsafe.Pointer
	value unsafe.Pointer // nil for receive; otherwise points to the send value
}

type tinygoChannelOp struct {
	next  unsafe.Pointer
	task  unsafe.Pointer
	index uint32
	value unsafe.Pointer
}

//go:linkname tinygoChanSelect runtime.chanSelect
func tinygoChanSelect(recvbuf unsafe.Pointer, states []tinygoSelectState, ops []tinygoChannelOp) (uint32, bool)

//go:linkname tinygoRandomIndex runtime.fastrandn
func tinygoRandomIndex(n uint32) uint32

func init() {
	if version := runtime.Version(); version != "0.42.0" {
		panic("bcast: select adapter requires TinyGo 0.42.0; got " + version)
	}
}

// selectCases implements the blocking send/receive subset of reflect.Select
// used by Bcast. A zero Chan disables a case, as does a typed nil channel.
// Default cases are not supported. Unlike reflect.Select, a received Value
// is addressable; Bcast only uses its Interface method.
//
// Blocking, channel locking, selecting exactly one operation, and removing
// unused waiters are all handled by TinyGo's existing select primitive.
//
// The panics use reflect's wording where the check is the same one reflect
// makes. What a version inside reflect would still need:
//
// TODO: support SelectDefault. The runtime already does non-blocking select,
// signalled by an empty ops slice, and reports no ready case as an index of
// ^uint32(0); reflect also rejects a second default, and a default carrying a
// Chan or Send value.
// TODO: return a non-addressable recv, as reflect does.
// TODO: match reflect's panics for a non-channel or unexported Chan, and for
// an unexported Send value, which reflect raises through mustBe and
// mustBeExported and which name the method and kind.
func selectCases(cases []reflect.SelectCase) (int, reflect.Value, bool) {
	if len(cases) > 65536 {
		panic("reflect.Select: too many cases (max 65536)")
	}
	if len(cases) == 0 {
		select {}
	}

	states := make([]tinygoSelectState, len(cases))
	order := make([]int, len(cases))
	var receiveType reflect.Type
	for i, c := range cases {
		order[i] = i
		if c.Dir == reflect.SelectDefault {
			panic("bcast: select adapter does not support default cases")
		}
		if c.Dir != reflect.SelectSend && c.Dir != reflect.SelectRecv {
			panic("reflect.Select: invalid Dir")
		}
		if c.Dir == reflect.SelectRecv && c.Send.IsValid() {
			panic("reflect.Select: RecvDir case has Send value")
		}
		if !c.Chan.IsValid() {
			continue
		}
		if c.Chan.Kind() != reflect.Chan || !c.Chan.CanInterface() {
			panic("reflect.Select: case Chan must be an exported channel")
		}
		typ := c.Chan.Type()
		states[i].ch = c.Chan.UnsafePointer()
		if c.Dir == reflect.SelectSend {
			if typ.ChanDir() == reflect.RecvDir {
				panic("reflect.Select: SendDir case using recv-only channel")
			}
			if !c.Send.IsValid() {
				panic("reflect.Select: SendDir case missing Send value")
			}
			if !c.Send.CanInterface() {
				panic("reflect.Select: SendDir case Send value must be exported")
			}
			// Set checks assignability and boxes concrete values when sending
			// to an interface channel. The state retains this allocation.
			storage := reflect.New(typ.Elem())
			storage.Elem().Set(c.Send)
			states[i].value = storage.UnsafePointer()
		} else {
			if typ.ChanDir() == reflect.SendDir {
				panic("reflect.Select: RecvDir case using send-only channel")
			}
			if receiveType == nil || typ.Elem().Size() > receiveType.Size() {
				receiveType = typ.Elem()
			}
		}
	}

	// TinyGo checks ready cases in slice order. Shuffle that order so a
	// continuously ready input cannot starve subscriber sends. Starting at
	// a random index would bias the choice when some cases are not ready.
	// This shuffle and the order slice exist only because chanSelect scans
	// in order; its own "TODO: start from a random index" is the better fix,
	// and doing it there would make ordinary select statements fair too.
	for i := len(states) - 1; i > 0; i-- {
		j := int(tinygoRandomIndex(uint32(i + 1)))
		states[i], states[j] = states[j], states[i]
		order[i], order[j] = order[j], order[i]
	}

	var receiveStorage reflect.Value
	var recvbuf unsafe.Pointer
	if receiveType != nil {
		// The runtime shares one receive buffer across all receive cases, so
		// receiveType is whichever element type is largest. TinyGo 0.42.0
		// implements reflect.New with runtime.alloc(size, nil), whose unknown
		// GC layout scans every word: it safely retains pointers even when
		// the selected element has a different layout from receiveType.
		//
		// Sizing by the largest type does not under-align the buffer for the
		// others, even though the largest need not be the most strictly
		// aligned. Every TinyGo allocator returns memory aligned for any Go
		// type: on wasm, align() in arch_tinygowasm.go rounds to 16, the
		// alignment of max_align_t, and the block GC's payload starts one
		// objHeader into a block of 4 words, so 8 bytes on a 32-bit target.
		receiveStorage = reflect.New(receiveType)
		recvbuf = receiveStorage.UnsafePointer()
	}
	// A nonempty ops slice tells the runtime to block when no case is ready.
	ops := make([]tinygoChannelOp, len(states))
	chosen, ok := tinygoChanSelect(recvbuf, states, ops)
	index := order[int(chosen)]
	var recv reflect.Value
	if c := cases[index]; c.Dir == reflect.SelectRecv {
		// Recover the selected element's actual type, not the type used to
		// size the shared buffer. New also uses an unknown GC layout here.
		recv = reflect.New(c.Chan.Type().Elem()).Elem()
		n := int(recv.Type().Size())
		copy(unsafe.Slice((*byte)(recv.Addr().UnsafePointer()), n), unsafe.Slice((*byte)(recvbuf), n))
	} else {
		if !ok {
			// A channel closed while this send was blocked. The runtime has
			// already removed all waiters, so it is safe to panic here.
			panic("send on closed channel")
		}
		ok = false // reflect.Select reports recvOK only for receive cases
	}
	// Keep channels, send storage, wait records, and the receive buffer alive
	// through runtime waiter cleanup and the copy into the typed result.
	runtime.KeepAlive(cases)
	runtime.KeepAlive(states)
	runtime.KeepAlive(ops)
	runtime.KeepAlive(receiveStorage)
	return index, recv, ok
}
