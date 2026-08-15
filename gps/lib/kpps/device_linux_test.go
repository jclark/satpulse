//go:build linux

package kpps

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFindDevice(t *testing.T) {
	dir := t.TempDir()
	write := func(name, path string) {
		if err := os.MkdirAll(filepath.Join(dir, name), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name, "path"), []byte(path), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("pps0", "\n")
	write("pps1", "/dev/ttyS9\n")
	write("pps2", "/dev/ttyS0\n")
	if err := os.MkdirAll(filepath.Join(dir, "pps3"), 0o755); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name      string
		source    string
		expect    string
		expectErr bool
	}{
		{name: "match", source: "/dev/ttyS0", expect: "pps2"},
		{name: "other tty", source: "/dev/ttyS9", expect: "pps1"},
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

// The sysfs attribute holds the canonical device path, so a descriptor opened
// through a symlink must still be matched by the path it resolves to. No PPS
// source can exist for a temporary file, so the failure names the path used.
func TestDevicePathForTTYResolvesSymlink(t *testing.T) {
	dir := t.TempDir()
	device := filepath.Join(dir, "ttyfake")
	if err := os.WriteFile(device, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(dir, "alias")
	if err := os.Symlink(device, alias); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(alias)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := f.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	})
	got, err := DevicePathForTTY(int(f.Fd()))
	if err == nil {
		t.Fatalf("DevicePathForTTY = %q, want an error", got)
	}
	if !strings.Contains(err.Error(), device) {
		t.Errorf("DevicePathForTTY error = %v, want it to name %q", err, device)
	}
}
