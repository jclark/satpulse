package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadMetadata(t *testing.T) {
	path := filepath.Join(t.TempDir(), "meta.json")
	data := `{"markerName":"FILE","receiver":{"type":"RX"},"comments":["from file"]}`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	cfg, _, err := parseArgs([]string{
		"--metadata", path,
		"--marker-name", "FLAG",
		"input.ubx",
		"output.obs",
	})
	if err != nil {
		t.Fatalf("parseArgs: %v", err)
	}
	meta, err := loadMetadata(cfg)
	if err != nil {
		t.Fatalf("loadMetadata: %v", err)
	}
	if meta.MarkerName != "FLAG" {
		t.Errorf("MarkerName = %q, want FLAG", meta.MarkerName)
	}
	if meta.Receiver.Type != "RX" {
		t.Errorf("Receiver.Type = %q, want RX", meta.Receiver.Type)
	}
	if len(meta.Comments) != 4 || meta.Comments[0] != "from file" || meta.Comments[1] != "format: u-blox UBX" || meta.Comments[2] != "options: -MULTICODE" || meta.Comments[3] != "log: input.ubx" {
		t.Errorf("Comments = %#v", meta.Comments)
	}
}
