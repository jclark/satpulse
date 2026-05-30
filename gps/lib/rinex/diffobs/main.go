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

type metadataDiffRecord struct {
	A rinex.Metadata `json:"a"`
	B rinex.Metadata `json:"b"`
}

type jsonReporter struct {
	enc *json.Encoder
}

func (r jsonReporter) Metadata(a, b rinex.Metadata) error {
	return r.enc.Encode(metadataDiffRecord{A: a, B: b})
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
	tol := rinex.Tolerances{
		Obs:      rinex.ObsTolerances{PR: 0.0005, CP: 0.0005, Do: 0.0005, CN0: 0.0005},
		Metadata: rinex.MetadataTolerances{ApproxPos: 0.00005, AntennaDelta: 0.00005},
	}
	flag.Float64Var(&tol.Obs.PR, "pr-tol", tol.Obs.PR, "pseudorange tolerance in meters")
	flag.Float64Var(&tol.Obs.CP, "cp-tol", tol.Obs.CP, "carrier phase tolerance in cycles")
	flag.Float64Var(&tol.Obs.Do, "do-tol", tol.Obs.Do, "Doppler tolerance in Hz")
	flag.Float64Var(&tol.Obs.CN0, "cn0-tol", tol.Obs.CN0, "C/N0 tolerance in dB-Hz")
	flag.Float64Var(&tol.Metadata.ApproxPos, "approx-pos-tol", tol.Metadata.ApproxPos, "APPROX POSITION XYZ tolerance in meters")
	flag.Float64Var(&tol.Metadata.AntennaDelta, "antenna-delta-tol", tol.Metadata.AntennaDelta, "ANTENNA: DELTA H/E/N tolerance in meters")
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "usage: diffobs [options] a.obs[.gz] b.obs[.gz]\n")
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() != 2 {
		flag.Usage()
		os.Exit(2)
	}
	if err := validateTolerances(tol); err != nil {
		fmt.Fprintln(os.Stderr, "diffobs: tolerances must be non-negative")
		os.Exit(2)
	}
	aMeta, aObs, err := readObservationFile(flag.Arg(0))
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", flag.Arg(0), err)
		os.Exit(2)
	}
	bMeta, bObs, err := readObservationFile(flag.Arg(1))
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", flag.Arg(1), err)
		os.Exit(2)
	}
	rep := jsonReporter{enc: json.NewEncoder(os.Stdout)}
	n := 0
	aOnly, bOnly := rinex.DiffMetadata(aMeta, bMeta, tol.Metadata)
	if !aOnly.IsZero() || !bOnly.IsZero() {
		if err := rep.Metadata(aOnly, bOnly); err != nil {
			fmt.Fprintf(os.Stderr, "diffobs: %v\n", err)
			os.Exit(2)
		}
		n++
	}
	m, err := rinex.DiffObservations(aObs, bObs, tol.Obs, rep, stderrErrorReporter{a: aObs, b: bObs})
	if err != nil {
		fmt.Fprintf(os.Stderr, "diffobs: %v\n", err)
		os.Exit(2)
	}
	n += m
	if n != 0 {
		os.Exit(1)
	}
}

func validateTolerances(tol rinex.Tolerances) error {
	if tol.Obs.PR < 0 || tol.Obs.CP < 0 || tol.Obs.Do < 0 || tol.Obs.CN0 < 0 ||
		tol.Metadata.ApproxPos < 0 || tol.Metadata.AntennaDelta < 0 {
		return fmt.Errorf("tolerances must be non-negative")
	}
	return nil
}

func sideName(side int) string {
	if side == 0 {
		return "a"
	}
	return "b"
}

func readObservationFile(path string) (rinex.Metadata, []rinex.SignalObservation, error) {
	r, err := openInput(path)
	if err != nil {
		return rinex.Metadata{}, nil, err
	}
	defer r.Close()
	return rinex.ReadObservationFile(r)
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
