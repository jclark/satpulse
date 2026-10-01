package bcast

import (
	"reflect"
	"strconv"
	"testing"
)

// Cases are built before timing to isolate the selector's allocation cost.
func BenchmarkSelectCases(b *testing.B) {
	for _, sends := range []int{0, 1, 4} {
		b.Run(strconv.Itoa(sends)+"Subscribers", func(b *testing.B) {
			value := makeSelectPayload(7)
			in := make(chan selectPayload, 1)
			in <- value
			var subscribe chan chan selectPayload
			var unsubscribe chan (<-chan selectPayload)
			var done <-chan struct{}
			cases := []reflect.SelectCase{
				{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(subscribe)},
				{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(unsubscribe)},
				{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(in)},
				{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(done)},
			}
			outputs := make([]chan selectPayload, sends)
			for i := range outputs {
				outputs[i] = make(chan selectPayload, 1)
				cases = append(cases, reflect.SelectCase{
					Dir:  reflect.SelectSend,
					Chan: reflect.ValueOf(outputs[i]),
					Send: reflect.ValueOf(&value).Elem(),
				})
			}
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				chosen, _, _ := selectCases(cases)
				if chosen == 2 {
					in <- value
				} else {
					<-outputs[chosen-4]
				}
			}
		})
	}
}
