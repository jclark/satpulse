package main

import "testing"

func TestCanOpenBrowser(t *testing.T) {
	tests := []struct {
		name string
		goos string
		env  map[string]string
		want bool
	}{
		{name: "linux desktop", goos: "linux", env: map[string]string{"DISPLAY": ":0"}, want: false},
		{name: "macOS", goos: "darwin", want: true},
		{name: "Windows", goos: "windows", want: true},
		{name: "SSH connection", goos: "darwin", env: map[string]string{"SSH_CONNECTION": "client server"}, want: false},
		{name: "SSH tty", goos: "windows", env: map[string]string{"SSH_TTY": "/dev/pts/0"}, want: false},
		{name: "unsupported", goos: "freebsd", want: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			getenv := func(name string) string { return tc.env[name] }
			if got := canOpenBrowser(tc.goos, getenv); got != tc.want {
				t.Errorf("got %t want %t", got, tc.want)
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
