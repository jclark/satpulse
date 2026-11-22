package main

import (
	"syscall/js"
	"testing"
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
