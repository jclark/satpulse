//go:build ignore
// +build ignore

// This program is not compiled normally. Run it explicitly with:
//
//	go run ./gps/lib/nmeamsg/check_corpus.go
package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/jclark/satpulse/gps/lib/nmeamsg"
)

type corpusRecord struct {
	Tag   string `json:"tag"`
	ASCII string `json:"ascii"`
}

type corpusStats struct {
	files     int
	records   int
	sentences int
	gga       int
	rmc       int
	failures  int
}

func main() {
	root := flag.String("root", "", "gps/testdata corpus root")
	maxErrors := flag.Int("max-errors", 20, "maximum errors to print")
	flag.Parse()
	if *root == "" {
		*root = defaultRoot()
	}
	var st corpusStats
	err := filepath.WalkDir(*root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || filepath.Ext(path) != ".jsonl" {
			return err
		}
		st.files++
		return checkFile(path, &st, *maxErrors)
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("checked %d GGA/RMC sentences (%d GGA, %d RMC) in %d records from %d files\n",
		st.sentences, st.gga, st.rmc, st.records, st.files)
	if st.failures != 0 {
		fmt.Fprintf(os.Stderr, "%d roundtrip failures\n", st.failures)
		os.Exit(1)
	}
}

func defaultRoot() string {
	for _, path := range []string{"gps/testdata", "../../testdata"} {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return "gps/testdata"
}

func checkFile(path string, st *corpusStats, maxErrors int) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for line := 1; sc.Scan(); line++ {
		st.records++
		var r corpusRecord
		if err := json.Unmarshal(sc.Bytes(), &r); err != nil {
			return fmt.Errorf("%s:%d: %w", path, line, err)
		}
		if r.Tag != "NMEA" {
			continue
		}
		format, ok := sentenceFormat(r.ASCII)
		if !ok {
			continue
		}
		st.sentences++
		switch format {
		case "GGA":
			st.gga++
		case "RMC":
			st.rmc++
		}
		if err := checkRoundtrip(r.ASCII); err != nil {
			st.failures++
			if st.failures <= maxErrors {
				fmt.Fprintf(os.Stderr, "%s:%d: %v\n", path, line, err)
			}
		}
	}
	return sc.Err()
}

func sentenceFormat(s string) (string, bool) {
	if nmeamsg.CheckSyntax(s)&nmeamsg.SentenceIsPacket == 0 {
		return "", false
	}
	if !strings.HasPrefix(s, "$") {
		return "", false
	}
	end := strings.IndexAny(s[1:], ",*")
	if end < 0 {
		return "", false
	}
	addr := s[1 : 1+end]
	if len(addr) != 5 {
		return "", false
	}
	format := addr[2:]
	return format, format == "GGA" || format == "RMC"
}

func checkRoundtrip(packet string) error {
	m1, err := parse(packet)
	if err != nil {
		return err
	}
	out, err := nmeamsg.SerializeMsg(m1)
	if err != nil {
		return fmt.Errorf("serialize: %w", err)
	}
	m2, err := parse(string(out))
	if err != nil {
		return fmt.Errorf("parse serialized %q: %w", out, err)
	}
	if m1.TalkerID() != m2.TalkerID() || m1.SentenceFormat() != m2.SentenceFormat() {
		return fmt.Errorf("address changed from %s%s to %s%s",
			m1.TalkerID(), m1.SentenceFormat(), m2.TalkerID(), m2.SentenceFormat())
	}
	if !reflect.DeepEqual(m1.Body(), m2.Body()) {
		return fmt.Errorf("body changed from %#v to %#v", m1.Body(), m2.Body())
	}
	return nil
}

func parse(packet string) (nmeamsg.GNSSTalkerIDMsg, error) {
	flags := nmeamsg.CheckSyntax(packet)
	if !flags.IsValidGNSSTalkerNMEA() {
		return nil, fmt.Errorf("invalid GNSS-talker NMEA syntax: %#x", flags)
	}
	star := strings.LastIndexByte(packet, '*')
	if star < 0 || star+3 > len(packet) {
		return nil, errors.New("missing checksum")
	}
	payload := packet[1:star]
	want := fmt.Sprintf("%02X", nmeamsg.Checksum([]byte(payload)))
	if got := packet[star+1 : star+3]; got != want {
		return nil, fmt.Errorf("checksum %s, want %s", got, want)
	}
	m, err := nmeamsg.ParseGNSSTalkerPayload(payload, flags)
	if err != nil {
		return nil, err
	}
	if m == nil {
		return nil, errors.New("not a GNSS-talker sentence")
	}
	return m, nil
}
