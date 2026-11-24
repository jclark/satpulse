//go:build linux

package kpps

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func writeDevice(t *testing.T, dir, name string, attrs map[string]string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, name), 0o755); err != nil {
		t.Fatal(err)
	}
	for attr, value := range attrs {
		if err := os.WriteFile(filepath.Join(dir, name, attr), []byte(value+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestListDevices(t *testing.T) {
	dir := t.TempDir()
	writeDevice(t, dir, "pps0", map[string]string{"name": "pps@12.-1", "path": "", "mode": "1151"})
	writeDevice(t, dir, "pps1", map[string]string{"name": "ttyAMA0", "path": "/dev/ttyAMA0", "mode": "1133"})
	writeDevice(t, dir, "pps2", map[string]string{"name": "broken"})
	got, err := listDevices(dir)
	if err != nil {
		t.Fatal(err)
	}
	expect := []Device{
		{Path: "/dev/pps0", Name: "pps@12.-1", Mode: 0x1151},
		{Path: "/dev/pps1", Name: "ttyAMA0", SourcePath: "/dev/ttyAMA0", Mode: 0x1133},
	}
	if !reflect.DeepEqual(got, expect) {
		t.Errorf("got  %+v\nwant %+v", got, expect)
	}
	if got, err := listDevices(filepath.Join(dir, "none")); err != nil || got != nil {
		t.Errorf("listDevices(missing dir) = %v, %v; want nil, nil", got, err)
	}
}

func TestFindDevice(t *testing.T) {
	dir := t.TempDir()
	for name, path := range map[string]string{"pps0": "", "pps1": "/dev/ttyS9", "pps2": "/dev/ttyS0"} {
		writeDevice(t, dir, name, map[string]string{"name": name, "path": path, "mode": "1111"})
	}
	if err := os.MkdirAll(filepath.Join(dir, "pps3"), 0o755); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name      string
		source    string
		expect    string
		expectErr bool
	}{
		{name: "match", source: "/dev/ttyS0", expect: "/dev/pps2"},
		{name: "other tty", source: "/dev/ttyS9", expect: "/dev/pps1"},
		{name: "no match", source: "/dev/ttyUSB0", expectErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := findDevice(dir, tc.source)
			if tc.expectErr {
				if err == nil {
					t.Fatalf("expected error, got %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.expect {
				t.Errorf("findDevice = %q, want %q", got, tc.expect)
			}
		})
	}
}
