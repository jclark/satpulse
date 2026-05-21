package main

import (
	"compress/gzip"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/jclark/satpulse/gps/lib/rinex"
)

type diffRecord struct {
	T   rinex.Time          `json:"t"`
	Sat rinex.SatelliteID   `json:"sat"`
	Sig rinex.SignalID      `json:"sig"`
	A   *rinex.SignalValues `json:"a,omitempty"`
	B   *rinex.SignalValues `json:"b,omitempty"`
}

type jsonReporter struct {
	enc *json.Encoder
}

func (r jsonReporter) Diff(t rinex.Time, sat rinex.SatelliteID, sig rinex.SignalID, a, b *rinex.SignalValues) error {
	rec := diffRecord{T: t, Sat: sat, Sig: sig}
	rec.A = a
	rec.B = b
	return r.enc.Encode(rec)
}

type stderrErrorReporter struct {
	a []rinex.SignalObservation
	b []rinex.SignalObservation
}

func (r stderrErrorReporter) Duplicate(side int, index, prevIndex int) error {
	obs := r.obs(side)
	o := obs[index]
	fmt.Fprintf(os.Stderr, "%s: duplicate observation at %s %s %s: observations %d and %d; keeping observation %d\n", sideName(side), o.T, o.Sat, o.Sig, prevIndex, index, prevIndex)
	return nil
}

func (r stderrErrorReporter) Unordered(side int, index int) error {
	obs := r.obs(side)
	fmt.Fprintf(os.Stderr, "%s: epoch out of order at observation %d: %s follows %s\n", sideName(side), index, obs[index].T, obs[index-1].T)
	return nil
}

func (r stderrErrorReporter) obs(side int) []rinex.SignalObservation {
	if side == 0 {
		return r.a
	}
	return r.b
}

func main() {
	tol := rinex.Tolerances{PR: 0.0005, CP: 0.0005, Do: 0.0005, CN0: 0.0005}
	flag.Float64Var(&tol.PR, "pr-tol", tol.PR, "pseudorange tolerance in meters")
	flag.Float64Var(&tol.CP, "cp-tol", tol.CP, "carrier phase tolerance in cycles")
	flag.Float64Var(&tol.Do, "do-tol", tol.Do, "Doppler tolerance in Hz")
	flag.Float64Var(&tol.CN0, "cn0-tol", tol.CN0, "C/N0 tolerance in dB-Hz")
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "usage: diffobs [options] a.obs[.gz] b.obs[.gz]\n")
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() != 2 {
		flag.Usage()
		os.Exit(2)
	}
	if tol.PR < 0 || tol.CP < 0 || tol.Do < 0 || tol.CN0 < 0 {
		fmt.Fprintln(os.Stderr, "diffobs: tolerances must be non-negative")
		os.Exit(2)
	}
	a, err := readObservations(flag.Arg(0))
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", flag.Arg(0), err)
		os.Exit(2)
	}
	b, err := readObservations(flag.Arg(1))
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", flag.Arg(1), err)
		os.Exit(2)
	}
	n, err := rinex.DiffObservations(a, b, tol, jsonReporter{enc: json.NewEncoder(os.Stdout)}, stderrErrorReporter{a: a, b: b})
	if err != nil {
		fmt.Fprintf(os.Stderr, "diffobs: %v\n", err)
		os.Exit(2)
	}
	if n != 0 {
		os.Exit(1)
	}
}

func sideName(side int) string {
	if side == 0 {
		return "a"
	}
	return "b"
}

func readObservations(path string) ([]rinex.SignalObservation, error) {
	r, err := openInput(path)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	_, obs, err := rinex.ReadObservationFile(r)
	return obs, err
}

func openInput(path string) (io.ReadCloser, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	if !strings.HasSuffix(path, ".gz") {
		return f, nil
	}
	zr, err := gzip.NewReader(f)
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	return inputCloser{Reader: zr, close: func() error {
		err := zr.Close()
		if err2 := f.Close(); err == nil {
			err = err2
		}
		return err
	}}, nil
}

type inputCloser struct {
	io.Reader
	close func() error
}

func (r inputCloser) Close() error {
	return r.close()
}
