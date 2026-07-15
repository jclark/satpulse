//go:build linux || freebsd

package main

import "testing"

func TestHasGraphicalSession(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		want bool
	}{
		{name: "X11", env: map[string]string{"DISPLAY": ":0"}, want: true},
		{name: "Wayland", env: map[string]string{"WAYLAND_DISPLAY": "wayland-0"}, want: true},
		{name: "headless", want: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("DISPLAY", tc.env["DISPLAY"])
			t.Setenv("WAYLAND_DISPLAY", tc.env["WAYLAND_DISPLAY"])
			if got := hasGraphicalSession(); got != tc.want {
				t.Errorf("got %t want %t", got, tc.want)
			}
		})
	}
}
