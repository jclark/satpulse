package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"math"
	"os"
	"path/filepath"
	"testing"
)

type Record struct {
	Chan      string      `json:"chan"`
	Timestamp json.Number `json:"timestamp"`
	QErr      json.Number `json:"qErr,omitempty"`
}

func (r *Record) timestampFloat() (float64, error) {
	return r.Timestamp.Float64()
}

func (r *Record) qerrFloat() (float64, error) {
	if r.QErr == "" {
		return 0, nil
	}
	return r.QErr.Float64()
}

func testTsgenOutput(t *testing.T, name string) {
	t.Helper()

	// Build paths
	tomlPath := filepath.Join("testdata", name+".toml")
	jsonlPath := filepath.Join("testdata", name+".jsonl")

	// Read expected output
	expectedFile, err := os.Open(jsonlPath)
	if err != nil {
		t.Fatalf("failed to open expected output: %v", err)
	}
	defer expectedFile.Close()

	// Run tsgen and capture output
	var gotBuf bytes.Buffer
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err = run([]string{tomlPath, "600"})
	w.Close()
	os.Stdout = oldStdout
	io.Copy(&gotBuf, r)

	if err != nil {
		t.Fatalf("tsgen failed: %v", err)
	}

	// Compare line by line
	expectedScanner := bufio.NewScanner(expectedFile)
	gotScanner := bufio.NewScanner(&gotBuf)

	lineNum := 0
	tolerance := 1e-12

	for expectedScanner.Scan() && gotScanner.Scan() {
		lineNum++

		var expected, got Record

		// Use decoder with UseNumber() to handle both string and numeric JSON
		expectedDec := json.NewDecoder(bytes.NewReader(expectedScanner.Bytes()))
		expectedDec.UseNumber()
		if err := expectedDec.Decode(&expected); err != nil {
			t.Fatalf("line %d: invalid expected JSON: %v", lineNum, err)
		}

		gotDec := json.NewDecoder(bytes.NewReader(gotScanner.Bytes()))
		gotDec.UseNumber()
		if err := gotDec.Decode(&got); err != nil {
			t.Fatalf("line %d: invalid got JSON: %v", lineNum, err)
		}

		// Compare channel
		if expected.Chan != got.Chan {
			t.Errorf("line %d: channel mismatch: expected %q, got %q",
				lineNum, expected.Chan, got.Chan)
		}

		// Parse and compare timestamp
		expectedTs, err := expected.timestampFloat()
		if err != nil {
			t.Fatalf("line %d: invalid expected timestamp %q: %v", lineNum, expected.Timestamp, err)
		}
		gotTs, err := got.timestampFloat()
		if err != nil {
			t.Fatalf("line %d: invalid got timestamp %q: %v", lineNum, got.Timestamp, err)
		}
		tsDiff := math.Abs(expectedTs - gotTs)
		if tsDiff > tolerance {
			t.Errorf("line %d: timestamp diff %.3e exceeds tolerance %.3e\n  expected: %.16f\n  got:      %.16f",
				lineNum, tsDiff, tolerance, expectedTs, gotTs)
		}

		// Compare qErr if present
		if expected.Chan == "B" {
			expectedQErr, err := expected.qerrFloat()
			if err != nil {
				t.Fatalf("line %d: invalid expected qErr %q: %v", lineNum, expected.QErr, err)
			}
			gotQErr, err := got.qerrFloat()
			if err != nil {
				t.Fatalf("line %d: invalid got qErr %q: %v", lineNum, got.QErr, err)
			}
			qerrDiff := math.Abs(expectedQErr - gotQErr)
			if qerrDiff > tolerance {
				t.Errorf("line %d: qErr diff %.3e exceeds tolerance %.3e\n  expected: %.16f\n  got:      %.16f",
					lineNum, qerrDiff, tolerance, expectedQErr, gotQErr)
			}
		}
	}

	// Check line counts match
	if expectedScanner.Scan() {
		t.Errorf("expected output has more lines than got (at least %d)", lineNum+1)
	}
	if gotScanner.Scan() {
		t.Errorf("got output has more lines than expected (at least %d)", lineNum+1)
	}

	t.Logf("✓ Validated %d lines within tolerance %.3e", lineNum, tolerance)
}

// PHC tests (4 types)
func TestPHCWhiteNoise(t *testing.T)   { testTsgenOutput(t, "phc.whiteNoise") }
func TestPHCFlickerNoise(t *testing.T) { testTsgenOutput(t, "phc.flickerNoise") }
func TestPHCRandomWalk(t *testing.T)   { testTsgenOutput(t, "phc.randomWalk") }
func TestPHCSinusoid(t *testing.T)     { testTsgenOutput(t, "phc.sinusoid") }

// GPS tests (4 types)
func TestGPSJitter(t *testing.T)     { testTsgenOutput(t, "gps.jitter") }
func TestGPSRandomWalk(t *testing.T) { testTsgenOutput(t, "gps.randomWalk") }
func TestGPSSawtooth(t *testing.T)   { testTsgenOutput(t, "gps.sawtooth") }
func TestGPSSinusoid(t *testing.T)   { testTsgenOutput(t, "gps.sinusoid") }
