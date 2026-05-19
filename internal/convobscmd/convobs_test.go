package convobscmd

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var updateGolden = flag.Bool("update", false, "update golden test data files")

func TestLoadMetadata(t *testing.T) {
	path := "meta.json"
	data := `{"markerName":"FILE","receiver":{"type":"RX"},"comments":["from file"]}`
	v, _, err := parseFlags("", []string{
		"--metadata", path,
		"--marker-name", "FLAG",
		"input.ubx",
	})
	if err != nil {
		t.Fatalf("parseFlags: %v", err)
	}
	if err := loadMetadata(&v.meta, v.metaPath, strings.NewReader(data)); err != nil {
		t.Fatalf("loadMetadata: %v", err)
	}
	meta := v.meta
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

func TestParseFlagsRequiresInput(t *testing.T) {
	_, _, err := parseFlags("", nil)
	if err == nil {
		t.Fatal("parseFlags succeeded without input")
	}
	v, _, err := parseFlags("", []string{"-", "-"})
	if err == nil {
		t.Fatal("parseFlags succeeded with positional output")
	}
	v, _, err = parseFlags("", []string{"-"})
	if err != nil {
		t.Fatalf("parseFlags dash input: %v", err)
	}
	if v.inputPath != "-" {
		t.Fatalf("inputPath = %q, want dash", v.inputPath)
	}
}

func TestGoldenFiles(t *testing.T) {
	// The golden files were checked against RTKLIB Explorer with:
	// convbin -r ubx -v 3.04 -od -os -ro -MULTICODE.
	// After normalizing PGM/RUN BY/DATE and the log path comment, the only
	// remaining header difference is that RTKLIB Explorer emits
	// SYS / PHASE SHIFT records. With those lines ignored, the output is
	// byte-identical to RTKLIB Explorer.
	now := time.Date(2026, time.May, 19, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		name string
		ubx  string
		obs  string
	}{
		{
			name: "m8t_20251217_4h",
			ubx:  filepath.Join("testdata", "m8t-20251217-4h.ubx"),
			obs:  filepath.Join("testdata", "m8t-20251217-4h.obs"),
		},
		{
			name: "f9t_20251217_3h",
			ubx:  filepath.Join("testdata", "f9t-20251217-3h.ubx"),
			obs:  filepath.Join("testdata", "f9t-20251217-3h.obs"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, _, err := parseFlags("", []string{tt.ubx})
			if err != nil {
				t.Fatalf("parseFlags: %v", err)
			}
			v.wopts.Date = now
			in, err := os.Open(v.inputPath)
			if err != nil {
				t.Fatalf("Open %s: %v", v.inputPath, err)
			}
			defer in.Close()
			var got bytes.Buffer
			if err := run(in, &got, v.meta, v.wopts, v.uopts); err != nil {
				t.Fatalf("run: %v", err)
			}
			if *updateGolden {
				if err := os.WriteFile(tt.obs, got.Bytes(), 0o644); err != nil {
					t.Fatalf("WriteFile %s: %v", tt.obs, err)
				}
			}
			want, err := os.ReadFile(tt.obs)
			if err != nil {
				t.Fatalf("ReadFile %s: %v", tt.obs, err)
			}
			if !bytes.Equal(got.Bytes(), want) {
				gotPath := filepath.Join(t.TempDir(), "got.obs")
				if err := os.WriteFile(gotPath, got.Bytes(), 0o644); err != nil {
					t.Fatalf("WriteFile %s: %v", gotPath, err)
				}
				t.Fatalf("output mismatch; got written to %s; %s", gotPath, firstDiff(got.Bytes(), want))
			}
		})
	}
}

func firstDiff(got, want []byte) string {
	gotLines := bytes.Split(got, []byte{'\n'})
	wantLines := bytes.Split(want, []byte{'\n'})
	n := min(len(gotLines), len(wantLines))
	for i := range n {
		if !bytes.Equal(gotLines[i], wantLines[i]) {
			return fmt.Sprintf("line %d differs\ngot:  %q\nwant: %q", i+1, gotLines[i], wantLines[i])
		}
	}
	return fmt.Sprintf("line count differs: got %d, want %d", len(gotLines), len(wantLines))
}
