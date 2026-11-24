//go:build linux && arm64

package gpiomem

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestProperty(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "status"), []byte("okay\x00"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "led"), 0o755); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name   string
		prop   string
		expect []byte
	}{
		{name: "present", prop: "status", expect: []byte("okay\x00")},
		{name: "absent", prop: "phandle"},
		{name: "child node", prop: "led"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := property(dir, tc.prop)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(got, tc.expect) {
				t.Errorf("got %q, want %q", got, tc.expect)
			}
		})
	}
}
