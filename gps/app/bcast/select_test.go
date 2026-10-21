package bcast

import (
	"reflect"
	"runtime"
	"testing"
	"time"
)

type selectNode struct{ Number int }

type selectPayload struct {
	Number int
	Text   string
	Bytes  []byte
	Ref    *selectNode
	Any    any
}

func makeSelectPayload(n int) selectPayload {
	return selectPayload{
		Number: n,
		Text:   string([]byte{'G', 'P', 'S', byte(n)}),
		Bytes:  []byte{byte(n), 2, 3},
		Ref:    &selectNode{n},
		Any:    &selectNode{n + 1},
	}
}

func checkSelectPayload(t *testing.T, p selectPayload, n int) {
	t.Helper()
	if p.Number != n || p.Text != string([]byte{'G', 'P', 'S', byte(n)}) ||
		!reflect.DeepEqual(p.Bytes, []byte{byte(n), 2, 3}) || p.Ref == nil ||
		p.Ref.Number != n || p.Any.(*selectNode).Number != n+1 {
		t.Fatalf("corrupted payload: %#v; want number %d", p, n)
	}
}

func receiveTestValue[T any](t *testing.T, ch <-chan T) (T, bool) {
	t.Helper()
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	select {
	case value, ok := <-ch:
		return value, ok
	case <-timer.C:
		t.Fatal("timed out waiting for channel")
		var zero T
		return zero, false
	}
}

func sendTestValue[T any](t *testing.T, ch chan<- T, value T) {
	t.Helper()
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	select {
	case ch <- value:
	case <-timer.C:
		t.Fatal("timed out sending to channel")
	}
}

func testSelectValue[T any](t *testing.T, value T) {
	t.Helper()
	in := make(chan T, 1)
	in <- value
	var disabled chan bool
	cases := []reflect.SelectCase{
		{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(disabled)},
		{Dir: reflect.SelectRecv}, // a zero Value disables this case
		{Dir: reflect.SelectRecv, Chan: reflect.ValueOf((<-chan T)(in))},
	}
	chosen, recv, ok := selectCases(cases)
	if chosen != 2 || !ok || recv.Type() != reflect.TypeOf(&value).Elem() || !reflect.DeepEqual(recv.Interface(), value) {
		t.Fatalf("receive = (%d, %v, %v); want case 2, %#v, true", chosen, recv, ok, value)
	}
	close(in)
	chosen, recv, ok = selectCases(cases)
	var zero T
	if chosen != 2 || ok || recv.Type() != reflect.TypeOf(&value).Elem() || !reflect.DeepEqual(recv.Interface(), zero) {
		t.Fatalf("closed receive = (%d, %v, %v); want case 2, %#v, false", chosen, recv, ok, zero)
	}

	out := make(chan T, 1)
	chosen, recv, ok = selectCases([]reflect.SelectCase{
		{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(disabled)},
		{Dir: reflect.SelectSend}, // disabled even without a send value
		{Dir: reflect.SelectSend, Chan: reflect.ValueOf((chan<- T)(out)), Send: reflect.ValueOf(&value).Elem()},
	})
	if chosen != 2 || recv.IsValid() || ok {
		t.Fatalf("send = (%d, %v, %v); want case 2, invalid Value, false", chosen, recv, ok)
	}
	got, _ := receiveTestValue(t, out)
	if !reflect.DeepEqual(got, value) {
		t.Fatalf("sent %#v; want %#v", got, value)
	}
}

func TestSelectValues(t *testing.T) {
	t.Run("bool", func(t *testing.T) { testSelectValue(t, true) })
	t.Run("byte", func(t *testing.T) { testSelectValue(t, byte(0xA5)) })
	t.Run("integer", func(t *testing.T) { testSelectValue(t, int(-42)) })
	t.Run("wideInteger", func(t *testing.T) { testSelectValue(t, uint64(0xFEDCBA9876543210)) })
	t.Run("zeroSize", func(t *testing.T) { testSelectValue(t, struct{}{}) })
	t.Run("payload", func(t *testing.T) { testSelectValue(t, makeSelectPayload(7)) })
	t.Run("slice", func(t *testing.T) { testSelectValue(t, []byte{1, 2, 3}) })
	t.Run("nilPointer", func(t *testing.T) { testSelectValue(t, (*selectNode)(nil)) })
	t.Run("interface", func(t *testing.T) { testSelectValue[any](t, makeSelectPayload(9)) })
	t.Run("nilInterface", func(t *testing.T) { testSelectValue[any](t, nil) })
	t.Run("channel", func(t *testing.T) { testSelectValue(t, make(chan selectPayload)) })
	t.Run("receiveOnlyChannel", func(t *testing.T) { testSelectValue(t, (<-chan selectPayload)(make(chan selectPayload))) })
}

func TestSelectConcreteSendToInterface(t *testing.T) {
	out := make(chan any, 1)
	chosen, recv, ok := selectCases([]reflect.SelectCase{
		{Dir: reflect.SelectSend, Chan: reflect.ValueOf(out), Send: reflect.ValueOf(makeSelectPayload(11))},
	})
	if chosen != 0 || recv.IsValid() || ok {
		t.Fatalf("send = (%d, %v, %v)", chosen, recv, ok)
	}
	got, _ := receiveTestValue(t, out)
	runtime.GC()
	checkSelectPayload(t, got.(selectPayload), 11)
}

func TestSelectMixedReceiveLayoutsAndGC(t *testing.T) {
	// A pointer-free type larger than the payload sizes the shared receive
	// buffer. The selected payload has pointers at different offsets.
	var large chan [32]uint64
	in := make(chan selectPayload, 1)
	for n := range 32 {
		in <- makeSelectPayload(n)
		chosen, recv, ok := selectCases([]reflect.SelectCase{
			{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(large)},
			{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(in)},
		})
		if chosen != 1 || !ok {
			t.Fatalf("receive = (%d, %v, %v)", chosen, recv, ok)
		}
		runtime.GC()
		checkSelectPayload(t, recv.Interface().(selectPayload), n)
	}
}

func TestSelectBlockedSendAndWaiterCleanup(t *testing.T) {
	a, b := make(chan selectPayload), make(chan selectPayload)
	started := make(chan struct{})
	finished := make(chan int, 1)
	go func() {
		cases := []reflect.SelectCase{
			{Dir: reflect.SelectSend, Chan: reflect.ValueOf(a), Send: reflect.ValueOf(makeSelectPayload(13))},
			{Dir: reflect.SelectSend, Chan: reflect.ValueOf(b), Send: reflect.ValueOf(makeSelectPayload(14))},
		}
		close(started)
		chosen, _, _ := selectCases(cases)
		finished <- chosen
	}()
	receiveTestValue(t, started)
	// Allow the selector to park with both send operations registered, then
	// collect while the runtime retains their buffers and wait records.
	time.Sleep(time.Millisecond)
	runtime.GC()
	got, ok := receiveTestValue(t, a)
	if !ok {
		t.Fatal("send output closed")
	}
	checkSelectPayload(t, got, 13)
	chosen, _ := receiveTestValue(t, finished)
	if chosen != 0 {
		t.Fatalf("selected case %d; want 0", chosen)
	}
	select {
	case <-b:
		t.Fatal("unselected send was left registered")
	default:
	}
	// Reuse the unchosen channel; a stale operation must not consume its
	// next receive or access the selector's old storage.
	go func() { b <- makeSelectPayload(15) }()
	got, _ = receiveTestValue(t, b)
	checkSelectPayload(t, got, 15)
}

func TestSelectBlockedReceiveLayoutsAndGC(t *testing.T) {
	var large chan [32]uint64
	in := make(chan selectPayload)
	started := make(chan struct{})
	finished := make(chan reflect.Value, 1)
	go func() {
		cases := []reflect.SelectCase{
			{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(large)},
			{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(in)},
		}
		close(started)
		chosen, recv, ok := selectCases(cases)
		if chosen != 1 || !ok {
			finished <- reflect.Value{}
			return
		}
		finished <- recv
	}()
	receiveTestValue(t, started)
	time.Sleep(time.Millisecond)
	runtime.GC()
	sendTestValue(t, in, makeSelectPayload(19))
	recv, _ := receiveTestValue(t, finished)
	if !recv.IsValid() {
		t.Fatal("blocked receive selected the wrong case")
	}
	runtime.GC()
	checkSelectPayload(t, recv.Interface().(selectPayload), 19)
}

func TestSelectBlockedReceiveAndDuplicateWaiterCleanup(t *testing.T) {
	in := make(chan selectPayload)
	done := make(chan struct{})
	started := make(chan struct{})
	finished := make(chan int, 1)
	go func() {
		cases := []reflect.SelectCase{
			{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(in)},
			{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(in)},
			{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(done)},
		}
		close(started)
		chosen, recv, ok := selectCases(cases)
		if chosen != 2 || ok || recv.Type() != reflect.TypeOf(struct{}{}) {
			finished <- -1
			return
		}
		finished <- chosen
	}()
	receiveTestValue(t, started)
	time.Sleep(time.Millisecond)
	runtime.GC()
	close(done)
	chosen, _ := receiveTestValue(t, finished)
	if chosen != 2 {
		t.Fatal("closed cancellation channel did not select case 2")
	}
	go func() { in <- makeSelectPayload(17) }()
	got, _ := receiveTestValue(t, in)
	checkSelectPayload(t, got, 17)
}

func TestSelectFairReadyCases(t *testing.T) {
	t.Run("receives", func(t *testing.T) { testSelectFairReadyCases(t, false) })
	t.Run("receiveAndSend", func(t *testing.T) { testSelectFairReadyCases(t, true) })
}

func testSelectFairReadyCases(t *testing.T, send bool) {
	t.Helper()
	ready := make(chan int, 1)
	ready <- 42
	out := make(chan int, 1)
	closed := make(chan struct{})
	close(closed)
	var disabled chan int
	cases := []reflect.SelectCase{
		{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(ready)},
		{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(disabled)},
		{Dir: reflect.SelectRecv},
		{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(closed)},
	}
	if send {
		cases[3] = reflect.SelectCase{Dir: reflect.SelectSend, Chan: reflect.ValueOf(out), Send: reflect.ValueOf(7)}
	}
	counts := [4]int{}
	for range 1024 {
		chosen, recv, ok := selectCases(cases)
		counts[chosen]++
		if chosen == 0 {
			if !ok || recv.Int() != 42 {
				t.Fatalf("ready receive = (%v, %v)", recv, ok)
			}
			ready <- 42
		} else {
			if chosen != 3 || ok {
				t.Fatalf("alternative = (%d, %v, %v)", chosen, recv, ok)
			}
			if send {
				if recv.IsValid() {
					t.Fatal("send returned a receive value")
				}
				if got := <-out; got != 7 {
					t.Fatalf("sent %d; want 7", got)
				}
			}
		}
	}
	// Broad bounds avoid relying on a particular random sequence while
	// catching deterministic case ordering and bias across disabled gaps.
	if counts[0] < 320 || counts[0] > 704 || counts[3] < 320 || counts[3] > 704 {
		t.Fatalf("unfair ready-case selection: %v", counts)
	}
}
