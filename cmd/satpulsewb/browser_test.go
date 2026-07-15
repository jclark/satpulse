package main

import "testing"

func TestCanOpenBrowser(t *testing.T) {
	tests := []struct {
		name string
		env  string
	}{
		{name: "SSH connection", env: "SSH_CONNECTION"},
		{name: "SSH tty", env: "SSH_TTY"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(tc.env, "set")
			if canOpenBrowser() {
				t.Error("got true want false")
			}
		})
	}
}

func TestLoopbackURL(t *testing.T) {
	tests := []struct {
		name  string
		token string
		want  string
	}{
		{name: "token", token: "abc", want: "http://127.0.0.1:15754/?t=abc"},
		{name: "no token", want: "http://127.0.0.1:15754/"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := loopbackURL(15754, tc.token); got != tc.want {
				t.Errorf("got %q want %q", got, tc.want)
			}
		})
	}
}
